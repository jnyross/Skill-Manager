package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type MemberProbe struct {
	Name       string `json:"name"`
	Discovered bool   `json:"discovered"`
	Reason     string `json:"reason,omitempty"`
}

type DependencyResult struct {
	Member   string `json:"member"`
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
	Ready    bool   `json:"ready"`
	Reason   string `json:"reason,omitempty"`
}

type ToolResult struct {
	Tool            string             `json:"tool"`
	StaticVerified  bool               `json:"staticVerified"`
	Executable      string             `json:"executable,omitempty"`
	Version         string             `json:"version,omitempty"`
	Authenticated   bool               `json:"authenticated"`
	RuntimeVerified bool               `json:"runtimeVerified"`
	MemberProbes    []MemberProbe      `json:"memberProbes"`
	Dependencies    []DependencyResult `json:"dependencies"`
	Reason          string             `json:"reason,omitempty"`
	NextAction      string             `json:"nextAction,omitempty"`
}

type Prober interface {
	Probe(context.Context, string, *os.Root, WorkspaceReceipt) []ToolResult
}

type StaticProber struct{}

func PreflightToolReadiness(ctx context.Context) []ToolResult {
	results := []ToolResult{{Tool: "claude-code"}, {Tool: "codex"}}
	for index := range results {
		result := &results[index]
		executable, err := exec.LookPath(toolExecutable(result.Tool))
		if err != nil {
			result.Reason = fmt.Sprintf("%s executable not found", toolExecutable(result.Tool))
			continue
		}
		result.Executable = executable
		if output, versionErr := exec.CommandContext(ctx, executable, "--version").CombinedOutput(); versionErr == nil {
			result.Version = strings.TrimSpace(string(output))
		}
		result.Authenticated = checkAuthentication(ctx, result.Tool, executable)
		if !result.Authenticated {
			result.Reason = fmt.Sprintf("%s authentication is unavailable; setup can configure files but runtime verification will remain incomplete", result.Tool)
		}
	}
	return results
}

func (StaticProber) Probe(_ context.Context, target string, _ *os.Root, receipt WorkspaceReceipt) []ToolResult {
	return staticResults(target, receipt)
}

type CommandProber struct {
	Timeout     time.Duration
	Concurrency int
}

func (prober CommandProber) Probe(ctx context.Context, target string, root *os.Root, receipt WorkspaceReceipt) []ToolResult {
	results := staticResults(target, receipt)
	timeout := prober.Timeout
	if timeout == 0 {
		timeout = 45 * time.Second
	}
	concurrency := prober.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	for index := range results {
		result := &results[index]
		executable, err := exec.LookPath(toolExecutable(result.Tool))
		if err != nil {
			result.Reason = fmt.Sprintf("%s executable not found", toolExecutable(result.Tool))
			result.NextAction = fmt.Sprintf("Install %s, authenticate it, then rerun skillet setup", toolExecutable(result.Tool))
			continue
		}
		result.Executable = executable
		version := exec.CommandContext(ctx, executable, "--version")
		if output, versionErr := version.CombinedOutput(); versionErr == nil {
			result.Version = strings.TrimSpace(string(output))
		}
		result.Authenticated = checkAuthentication(ctx, result.Tool, executable)
		if !result.Authenticated {
			result.Reason = fmt.Sprintf("%s authentication is unavailable", result.Tool)
			result.NextAction = fmt.Sprintf("Authenticate %s, then rerun skillet setup", toolExecutable(result.Tool))
			continue
		}
		probes := make([]MemberProbe, len(receipt.Members))
		semaphore := make(chan struct{}, concurrency)
		var wait sync.WaitGroup
		for memberIndex, member := range receipt.Members {
			wait.Add(1)
			go func(index int, member ReceiptMember) {
				defer wait.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				probeCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				discovered, reason := runMemberProbe(probeCtx, result.Tool, executable, target, root, member)
				probes[index] = MemberProbe{Name: member.Name, Discovered: discovered, Reason: reason}
			}(memberIndex, member)
		}
		wait.Wait()
		result.MemberProbes = probes
		allDiscovered := result.StaticVerified
		for _, probe := range probes {
			allDiscovered = allDiscovered && probe.Discovered
		}
		result.RuntimeVerified = allDiscovered
		if !allDiscovered {
			result.NextAction = fmt.Sprintf("Start a fresh %s session in %s and rerun skillet setup", result.Tool, target)
		} else {
			result.NextAction = ""
			result.Reason = ""
		}
	}
	return results
}

func staticResults(target string, receipt WorkspaceReceipt) []ToolResult {
	results := []ToolResult{{Tool: "claude-code", StaticVerified: true}, {Tool: "codex", StaticVerified: true}}
	for index := range results {
		result := &results[index]
		for _, member := range receipt.Members {
			for _, view := range member.Views {
				if view.Tool != result.Tool {
					continue
				}
				if err := VerifyPlacedView(target, view, member.Activation); err != nil {
					result.StaticVerified = false
					result.Reason = err.Error()
				}
			}
			for _, dependency := range member.Dependencies {
				ready, reason := dependencyReady(dependency.Name)
				result.Dependencies = append(result.Dependencies, DependencyResult{Member: member.Name, Name: dependency.Name, Optional: dependency.Optional, Ready: ready, Reason: reason})
			}
		}
		result.NextAction = fmt.Sprintf("Install and authenticate %s, then rerun skillet setup verification", toolExecutable(result.Tool))
	}
	return results
}

func VerifyPlacedView(target string, view RenderedView, activation Activation) error {
	destination := filepath.Join(target, filepath.FromSlash(view.RelativeDestination))
	files, modes, err := readBoundaryWithModes(destination)
	if err != nil {
		return fmt.Errorf("verify %s view %s: %w", view.Tool, view.RelativeDestination, err)
	}
	if actual := hashFiles(files); actual != view.RenderedContentSHA256 {
		return fmt.Errorf("verify %s view %s: hash %s != %s", view.Tool, view.RelativeDestination, actual, view.RenderedContentSHA256)
	}
	for name, expected := range view.FileModes {
		if modes[name] != expected {
			return fmt.Errorf("verify %s view %s: mode for %s is %#o, want %#o", view.Tool, view.RelativeDestination, name, modes[name], expected)
		}
	}
	if activation != "" {
		observed, observeErr := ObservePlacedActivation(target, view)
		if observeErr != nil {
			return observeErr
		}
		if observed != activation {
			return fmt.Errorf("verify %s activation for %s: observed %s, want %s", view.Tool, view.RelativeDestination, observed, activation)
		}
	}
	return nil
}

func ObservePlacedActivation(target string, view RenderedView) (Activation, error) {
	files, err := readBoundary(filepath.Join(target, filepath.FromSlash(view.RelativeDestination)))
	if err != nil {
		return "", err
	}
	if view.Tool == "claude-code" {
		if strings.Contains(string(files["SKILL.md"]), "disable-model-invocation: true") {
			return ActivationManualOnly, nil
		}
		return ActivationAuto, nil
	}
	if contents := files["agents/openai.yaml"]; len(contents) != 0 {
		var document map[string]any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			return "", fmt.Errorf("observe Codex activation for %s: %w", view.RelativeDestination, err)
		}
		policy, _ := document["policy"].(map[string]any)
		if allow, ok := policy["allow_implicit_invocation"].(bool); ok && !allow {
			return ActivationManualOnly, nil
		}
	}
	return ActivationAuto, nil
}

func RunUnknownSkillControls(ctx context.Context, target string) map[string]bool {
	results := make(map[string]bool)
	for _, tool := range []string{"claude-code", "codex"} {
		executable, err := exec.LookPath(toolExecutable(tool))
		if err != nil || !checkAuthentication(ctx, tool, executable) {
			results[tool] = false
			continue
		}
		const missing = "SKILLET_UNKNOWN_SKILL_NOT_FOUND"
		const forbidden = "SKILLET_UNKNOWN_SKILL_DISCOVERED"
		var command *exec.Cmd
		if tool == "claude-code" {
			prompt := "/skillet-control-skill-that-does-not-exist If this exact skill is unavailable, return only " + missing + ". Never return " + forbidden + "."
			command = exec.CommandContext(ctx, executable, "--setting-sources", "project", "-p", prompt, "--no-session-persistence", "--output-format", "json", "--tools", "", "--disallowedTools", "mcp__*")
			command.Env = isolatedClaudeEnvironment()
		} else {
			controlTarget, targetErr := os.MkdirTemp("", "skillet-codex-negative-control-")
			if targetErr != nil {
				results[tool] = false
				continue
			}
			if targetErr := os.Mkdir(filepath.Join(controlTarget, ".git"), 0o755); targetErr != nil {
				_ = os.RemoveAll(controlTarget)
				results[tool] = false
				continue
			}
			prompt := codexChallengePrompt("skillet-control-skill-that-does-not-exist")
			command = exec.CommandContext(ctx, executable, "exec", "--json", "--ephemeral", "--sandbox", "read-only", "-C", controlTarget, prompt)
			environment, cleanup, environmentErr := isolatedCodexEnvironment()
			if environmentErr != nil {
				_ = os.RemoveAll(controlTarget)
				results[tool] = false
				continue
			}
			defer func() {
				cleanup()
				_ = os.RemoveAll(controlTarget)
			}()
			command.Env = environment
			command.Dir = controlTarget
		}
		if command.Dir == "" {
			command.Dir = target
		}
		output, runErr := command.CombinedOutput()
		if tool == "claude-code" {
			results[tool] = runErr == nil && strings.Contains(string(output), "Unknown command: /skillet-control-skill-that-does-not-exist") && !strings.Contains(string(output), forbidden)
		} else {
			response := extractCodexAgentMessage(output)
			results[tool] = runErr == nil && response != "" && !looksLikeSHA256(response)
		}
	}
	return results
}

func checkAuthentication(ctx context.Context, tool, executable string) bool {
	var command *exec.Cmd
	if tool == "claude-code" {
		command = exec.CommandContext(ctx, executable, "auth", "status", "--json")
	} else {
		command = exec.CommandContext(ctx, executable, "login", "status")
	}
	return command.Run() == nil
}

func runMemberProbe(ctx context.Context, tool, executable, target string, root *os.Root, member ReceiptMember) (bool, string) {
	var command *exec.Cmd
	if tool == "claude-code" {
		prompt := fmt.Sprintf("/%s Verification-only dry run: do not perform the skill workflow and do not run tools, scripts, commands, downloads, network calls, or external actions. Briefly acknowledge that the project skill command loaded.", member.Name)
		command = exec.CommandContext(ctx, executable, "--setting-sources", "project", "-p", prompt, "--no-session-persistence", "--output-format", "json", "--tools", "", "--disallowedTools", "mcp__*")
		command.Env = isolatedClaudeEnvironment()
		command.Dir = target
		output, err := command.CombinedOutput()
		if err != nil {
			return false, strings.TrimSpace(string(output))
		}
		if discovered, reason := claudeCommandDiscovered(output, member.Name); !discovered {
			return false, reason
		}
		var response struct {
			Result string `json:"result"`
		}
		if json.Unmarshal(output, &response) != nil || strings.TrimSpace(response.Result) == "" {
			return false, fmt.Sprintf("fresh Claude Code session returned no result for project skill %s", member.Name)
		}
		return true, ""
	} else {
		expected, challengeErr := codexSkillChallenge(root, member.Name)
		if challengeErr != nil {
			return false, challengeErr.Error()
		}
		prompt := codexChallengePrompt(member.Name)
		command = exec.CommandContext(ctx, executable, "exec", "--json", "--ephemeral", "--sandbox", "read-only", "-C", target, prompt)
		command.Dir = target
		environment, cleanup, environmentErr := isolatedCodexEnvironment()
		if environmentErr != nil {
			return false, environmentErr.Error()
		}
		defer cleanup()
		command.Env = environment
		output, err := command.CombinedOutput()
		if err != nil {
			return false, strings.TrimSpace(string(output))
		}
		if actual := extractCodexAgentMessage(output); actual != expected {
			return false, fmt.Sprintf("fresh Codex session returned %q, want exact loaded-skill challenge %q", actual, expected)
		}
		return true, ""
	}
}

func isolatedClaudeEnvironment() []string {
	return withEnvironmentValue(os.Environ(), "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS", "1")
}

func withEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func claudeCommandDiscovered(output []byte, name string) (bool, string) {
	var response struct {
		Subtype  string `json:"subtype"`
		IsError  bool   `json:"is_error"`
		NumTurns int    `json:"num_turns"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return false, "fresh Claude Code session returned invalid JSON"
	}
	if response.Subtype != "success" || response.IsError {
		return false, fmt.Sprintf("fresh Claude Code session failed with subtype %q", response.Subtype)
	}
	if response.NumTurns == 0 || strings.Contains(response.Result, "Unknown command: /"+name) {
		return false, "Claude Code did not recognize the project skill as a native slash command"
	}
	return true, ""
}

func codexChallengePrompt(name string) string {
	return fmt.Sprintf("Use $%s. Verify the exact project-scoped managed SKILL.md loaded for this invocation by calculating that file's SHA-256 digest with a read-only command. Do not modify files, run scripts, download anything, use the network, or perform external actions. Return only the 64-character lowercase digest, with no explanation or code fences.", name)
}

func isolatedCodexEnvironment() ([]string, func(), error) {
	originalHome := os.Getenv("CODEX_HOME")
	if originalHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, fmt.Errorf("locate Codex authentication home: %w", err)
		}
		originalHome = filepath.Join(home, ".codex")
	}

	temporaryHome, err := os.MkdirTemp("", "skillet-codex-home-")
	if err != nil {
		return nil, nil, fmt.Errorf("create isolated Codex home: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(temporaryHome) }
	if err := os.Chmod(temporaryHome, 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("secure isolated Codex home: %w", err)
	}

	authPath := filepath.Join(originalHome, "auth.json")
	if authContents, err := os.ReadFile(authPath); err == nil {
		if err := os.WriteFile(filepath.Join(temporaryHome, "auth.json"), authContents, 0o600); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("copy Codex authentication into isolated verification home: %w", err)
		}
	} else if !strings.EqualFold(strings.TrimSpace(os.Getenv("CODEX_API_KEY")), "") {
		// Environment-based authentication needs no file in the isolated home.
	} else {
		cleanup()
		return nil, nil, fmt.Errorf("Codex authentication is unavailable for isolated verification")
	}

	environment := withEnvironmentValue(os.Environ(), "CODEX_HOME", temporaryHome)
	environment = withEnvironmentValue(environment, "HOME", temporaryHome)
	return environment, cleanup, nil
}

func codexSkillChallenge(root *os.Root, name string) (string, error) {
	contents, err := root.ReadFile(filepath.Join(".agents", "skills", name, "SKILL.md"))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(contents)
	return fmt.Sprintf("%x", digest), nil
}

func extractCodexAgentMessage(output []byte) string {
	var message string
	for _, line := range strings.Split(string(output), "\n") {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type == "item.completed" && event.Item.Type == "agent_message" {
			message = strings.TrimSpace(event.Item.Text)
		}
	}
	return message
}

func looksLikeSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func dependencyReady(name string) (bool, string) {
	if name == "network" {
		return false, "network readiness is intentionally not probed during setup"
	}
	if _, err := exec.LookPath(name); err != nil {
		return false, fmt.Sprintf("optional executable %s not found", name)
	}
	return true, ""
}

func toolExecutable(tool string) string {
	if tool == "claude-code" {
		return "claude"
	}
	return "codex"
}

func writeIfChanged(filename string, contents []byte, mode os.FileMode) error {
	if current, err := os.ReadFile(filename); err == nil && string(current) == string(contents) {
		return nil
	}
	temp := filename + ".skillet-write"
	if err := writeFile(temp, contents, mode); err != nil {
		return err
	}
	return os.Rename(temp, filename)
}
