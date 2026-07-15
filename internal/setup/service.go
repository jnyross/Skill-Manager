package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jnyross/Skill-Manager/internal/catalog"
)

type Outcome string

const (
	OutcomeBlocked              Outcome = "Blocked"
	OutcomeConfiguredUnverified Outcome = "Configured-unverified"
	OutcomeVerified             Outcome = "Verified"
	OutcomePartial              Outcome = "Partial"
)

type Request struct {
	TargetPath       string
	CatalogVersion   string
	BundleIDs        []string
	Members          []ResolvedMember
	Activation       map[string]Activation
	AcceptDrift      map[string]bool
	ReplaceConflicts bool
}

type ChangeState string

const (
	ChangeAbsent           ChangeState = "create"
	ChangeExactAdoption    ChangeState = "adopt-exact"
	ChangeManagedUnchanged ChangeState = "managed-unchanged"
	ChangeManagedUpdate    ChangeState = "managed-update"
	ChangeManagedRemove    ChangeState = "managed-remove"
	ChangeEditedManaged    ChangeState = "edited-managed"
	ChangeEditedRemoval    ChangeState = "edited-managed-remove"
	ChangeConflict         ChangeState = "unmanaged-conflict"
	ChangeMerge            ChangeState = "merge-preserving-user-content"
)

type PlannedChange struct {
	Path        string      `json:"path"`
	State       ChangeState `json:"state"`
	BeforeHash  string      `json:"beforeHash,omitempty"`
	DesiredHash string      `json:"desiredHash"`
	BeforeMode  uint32      `json:"beforeMode,omitempty"`
	DesiredMode uint32      `json:"desiredMode"`
}

type Plan struct {
	TargetPath       string          `json:"targetPath"`
	CatalogVersion   string          `json:"catalogVersion"`
	BundleIDs        []string        `json:"bundleIds"`
	Changes          []PlannedChange `json:"changes"`
	Blockers         []string        `json:"blockers,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
	NoOp             bool            `json:"noOp"`
	NeedGitInit      bool            `json:"needGitInit"`
	ReplaceConflicts bool            `json:"replaceConflicts"`
	desired          map[string][]byte
	desiredModes     map[string]os.FileMode
	targetIdentity   os.FileInfo
}

type Result struct {
	Outcome          Outcome  `json:"outcome"`
	ReceiptPath      string   `json:"receiptPath,omitempty"`
	LocalReceiptPath string   `json:"localReceiptPath,omitempty"`
	Backups          []string `json:"backups,omitempty"`
	NextAction       string   `json:"nextAction,omitempty"`
	NoOp             bool     `json:"noOp"`
}

type ManagedPath struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}

type ReceiptMember struct {
	Name               string               `json:"name"`
	ReviewedRevision   string               `json:"reviewedRevision"`
	ResolvedRevision   string               `json:"resolvedRevision"`
	ResolvedSHA256     string               `json:"resolvedSHA256"`
	Drift              DriftReview          `json:"drift"`
	DriftAccepted      bool                 `json:"driftAccepted"`
	Activation         Activation           `json:"activation"`
	Views              []RenderedView       `json:"views"`
	VerificationPrompt string               `json:"verificationPrompt"`
	Dependencies       []catalog.Dependency `json:"dependencies,omitempty"`
	ExternalActions    []string             `json:"externalActions,omitempty"`
}

type WorkspaceReceipt struct {
	SchemaVersion      int                 `json:"schemaVersion"`
	CatalogVersion     string              `json:"catalogVersion"`
	BundleIDs          []string            `json:"bundleIds"`
	Members            []ReceiptMember     `json:"members"`
	ManagedPaths       []ManagedPath       `json:"managedPaths"`
	Outcome            Outcome             `json:"outcome"`
	Previous           *ReceiptIdentity    `json:"previous,omitempty"`
	Decisions          []PlannedChange     `json:"decisions,omitempty"`
	Backups            []string            `json:"backups,omitempty"`
	History            []ReceiptTransition `json:"history,omitempty"`
	LocalReceiptSHA256 string              `json:"localReceiptSHA256,omitempty"`
}

type ReceiptIdentity struct {
	CatalogVersion string   `json:"catalogVersion"`
	BundleIDs      []string `json:"bundleIds"`
	Outcome        Outcome  `json:"outcome"`
}

type ReceiptTransition struct {
	CatalogVersion string                  `json:"catalogVersion"`
	BundleIDs      []string                `json:"bundleIds"`
	Outcome        Outcome                 `json:"outcome"`
	Members        []ReceiptMemberIdentity `json:"members"`
	Decisions      []PlannedChange         `json:"decisions,omitempty"`
	Backups        []string                `json:"backups,omitempty"`
}

type ReceiptMemberIdentity struct {
	Name             string      `json:"name"`
	ReviewedRevision string      `json:"reviewedRevision"`
	ResolvedRevision string      `json:"resolvedRevision"`
	ResolvedSHA256   string      `json:"resolvedSHA256"`
	Activation       Activation  `json:"activation"`
	Drift            DriftReview `json:"drift"`
	DriftAccepted    bool        `json:"driftAccepted"`
}

type LocalReceipt struct {
	SchemaVersion         int          `json:"schemaVersion"`
	AbsoluteTarget        string       `json:"absoluteTarget"`
	Outcome               Outcome      `json:"outcome"`
	ToolResults           []ToolResult `json:"toolResults"`
	PortableReceiptSHA256 string       `json:"portableReceiptSHA256,omitempty"`
	SelfSHA256            string       `json:"selfSHA256,omitempty"`
}

type Service struct {
	adapters []ToolAdapter
	prober   Prober
	hooks    ApplyHooks
}

type ApplyHooks struct {
	BeforePromote  func(relativePath string) error
	ExternalAction func(context.Context, string) (undo func() error, repairAction string, err error)
}

func NewService() *Service {
	return &Service{adapters: []ToolAdapter{NewClaudeAdapter(), NewCodexAdapter()}, prober: StaticProber{}}
}

func NewLiveService() *Service {
	return &Service{adapters: []ToolAdapter{NewClaudeAdapter(), NewCodexAdapter()}, prober: CommandProber{}}
}

func NewServiceWith(prober Prober, hooks ApplyHooks) *Service {
	if prober == nil {
		prober = StaticProber{}
	}
	return &Service{adapters: []ToolAdapter{NewClaudeAdapter(), NewCodexAdapter()}, prober: prober, hooks: hooks}
}

func (service *Service) Plan(_ context.Context, request Request) (Plan, error) {
	target, needGit, err := normalizeTarget(request.TargetPath)
	if err != nil {
		return Plan{}, err
	}
	desired := make(map[string][]byte)
	desiredModes := make(map[string]os.FileMode)
	warnings := make([]string, 0)
	receiptMembers := make([]ReceiptMember, 0, len(request.Members))
	prior, priorErr := loadPriorReceipt(target)
	if priorErr != nil {
		warnings = append(warnings, priorErr.Error())
	}
	if priorErr == nil {
		if err := validatePriorReceipt(prior); err != nil {
			return Plan{}, err
		}
	}
	priorLocal, localErr := loadLocalReceipt(target)
	if localErr != nil {
		return Plan{}, localErr
	}
	if err := validateReceiptPair(target, prior, priorLocal); err != nil {
		return Plan{}, err
	}

	agents := "# Managed by Skillet\n\nThis Agent-ready workspace uses the shared project skills selected from Built-in catalog " + request.CatalogVersion + ".\n"
	desired["AGENTS.md"] = []byte(agents)
	desired["CLAUDE.md"] = []byte("# Managed by Skillet\n\n@AGENTS.md\n")
	desiredModes["AGENTS.md"] = 0o644
	desiredModes["CLAUDE.md"] = 0o644

	for _, resolved := range request.Members {
		if resolved.Drift.Material && !request.AcceptDrift[resolved.Member.Name] {
			warnings = append(warnings, fmt.Sprintf("%s has material latest-source drift that was not accepted", resolved.Member.Name))
		}
		activation := Activation(resolved.Member.UpstreamActivation)
		if activation == "" {
			activation = ActivationAuto
		}
		override, hasOverride := request.Activation[resolved.Member.Name]
		if hasOverride {
			activation = override
		}
		canonicalFiles, canonicalModes, err := readBoundaryWithModes(resolved.SourceDir)
		if err != nil {
			return Plan{}, fmt.Errorf("read canonical core %s: %w", resolved.Member.Name, err)
		}
		for name, contents := range canonicalFiles {
			destination := filepath.ToSlash(filepath.Join(".skillet", "managed", "skills", resolved.Member.Name, filepath.FromSlash(name)))
			desired[destination] = contents
			desiredModes[destination] = os.FileMode(canonicalModes[name])
		}
		receiptMember := ReceiptMember{
			Name: resolved.Member.Name, ReviewedRevision: resolved.Member.Source.ReviewedRevision,
			ResolvedRevision: resolved.Evidence.Revision, ResolvedSHA256: resolved.Evidence.ContentSHA256,
			Drift: resolved.Drift, DriftAccepted: resolved.Drift.Material && request.AcceptDrift[resolved.Member.Name], Activation: activation,
			VerificationPrompt: resolved.Member.VerificationPrompt,
			Dependencies:       resolved.Member.Dependencies, ExternalActions: resolved.Member.ExternalActions,
		}
		for _, adapter := range service.adapters {
			view, err := adapter.Render(RenderRequest{Member: resolved.Member, SourceDir: resolved.SourceDir, Activation: activation, ActivationOverride: hasOverride})
			if err != nil {
				return Plan{}, err
			}
			receiptMember.Views = append(receiptMember.Views, view)
			warnings = append(warnings, view.Warnings...)
			for name, contents := range view.Files {
				destination := filepath.ToSlash(filepath.Join(filepath.FromSlash(view.RelativeDestination), filepath.FromSlash(name)))
				desired[destination] = contents
				desiredModes[destination] = os.FileMode(view.FileModes[name])
			}
		}
		receiptMembers = append(receiptMembers, receiptMember)
	}

	ignorePath := filepath.Join(target, ".gitignore")
	ignoreContents, readErr := os.ReadFile(ignorePath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return Plan{}, fmt.Errorf("read .gitignore: %w", readErr)
	}
	desired[".gitignore"] = mergeIgnore(ignoreContents, ".skillet/workspace.local.json")
	desiredModes[".gitignore"] = 0o644

	managed := make([]ManagedPath, 0, len(desired))
	for name, contents := range desired {
		mode := desiredModes[name]
		if mode == 0 {
			mode = 0o644
		}
		managed = append(managed, ManagedPath{Path: name, SHA256: hashBytes(contents), Mode: uint32(mode.Perm())})
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].Path < managed[j].Path })
	receiptOutcome := OutcomeConfiguredUnverified
	if prior.Outcome != "" {
		receiptOutcome = prior.Outcome
	}
	managed = append(managed,
		ManagedPath{Path: ".skillet/workspace.json", Mode: 0o644},
		ManagedPath{Path: ".skillet/workspace.local.json", Mode: 0o644},
	)
	sort.Slice(managed, func(i, j int) bool { return managed[i].Path < managed[j].Path })
	receipt := WorkspaceReceipt{
		SchemaVersion: 1, CatalogVersion: request.CatalogVersion, BundleIDs: append([]string(nil), request.BundleIDs...),
		Members: receiptMembers, ManagedPaths: managed, Outcome: receiptOutcome,
		Decisions: append([]PlannedChange(nil), prior.Decisions...), Backups: append([]string(nil), prior.Backups...),
		History: append([]ReceiptTransition(nil), prior.History...),
	}
	if prior.CatalogVersion != "" && !receiptConfigurationEqual(prior, receipt) {
		receipt.History = append(receipt.History, snapshotReceipt(prior))
	}
	if prior.CatalogVersion != "" && (prior.CatalogVersion != request.CatalogVersion || !stringSlicesEqual(prior.BundleIDs, request.BundleIDs)) {
		receipt.Previous = &ReceiptIdentity{CatalogVersion: prior.CatalogVersion, BundleIDs: append([]string(nil), prior.BundleIDs...), Outcome: prior.Outcome}
	}
	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Plan{}, err
	}
	desired[".skillet/workspace.json"] = append(receiptBytes, '\n')
	desiredModes[".skillet/workspace.json"] = 0o644
	local := LocalReceipt{SchemaVersion: 1, AbsoluteTarget: target, Outcome: OutcomeConfiguredUnverified, ToolResults: []ToolResult{}}
	if priorLocal.SchemaVersion == 1 {
		local = priorLocal
	}
	localBytes, err := json.MarshalIndent(local, "", "  ")
	if err != nil {
		return Plan{}, err
	}
	desired[".skillet/workspace.local.json"] = append(localBytes, '\n')
	desiredModes[".skillet/workspace.local.json"] = 0o644
	if existingLocal, readErr := os.ReadFile(filepath.Join(target, ".skillet", "workspace.local.json")); readErr == nil && priorLocal.SchemaVersion == 1 {
		desired[".skillet/workspace.local.json"] = existingLocal
	}

	priorHashes := make(map[string]string)
	priorModes := make(map[string]uint32)
	for _, managedPath := range prior.ManagedPaths {
		priorHashes[managedPath.Path] = managedPath.SHA256
		priorModes[managedPath.Path] = managedPath.Mode
	}
	for _, receiptPath := range []string{".skillet/workspace.json", ".skillet/workspace.local.json"} {
		if _, owned := priorHashes[receiptPath]; owned {
			if contents, readErr := os.ReadFile(filepath.Join(target, filepath.FromSlash(receiptPath))); readErr == nil {
				priorHashes[receiptPath] = hashBytes(contents)
			}
		}
	}

	plan := Plan{
		TargetPath: target, CatalogVersion: request.CatalogVersion, BundleIDs: append([]string(nil), request.BundleIDs...),
		NeedGitInit: needGit, ReplaceConflicts: request.ReplaceConflicts, desired: desired, desiredModes: desiredModes, Warnings: warnings,
	}
	if info, statErr := os.Stat(target); statErr == nil {
		plan.targetIdentity = info
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "material latest-source drift") || strings.Contains(warning, "parse prior workspace receipt") {
			plan.Blockers = append(plan.Blockers, warning)
		}
	}
	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)
	changed := needGit
	for _, name := range names {
		if blocker := destinationSymlinkBlocker(target, name); blocker != "" {
			plan.Blockers = append(plan.Blockers, blocker)
			changed = true
			continue
		}
		contents := desired[name]
		change := classifyChange(target, name, contents, desiredModes[name], priorHashes, priorModes)
		plan.Changes = append(plan.Changes, change)
		switch change.State {
		case ChangeAbsent, ChangeManagedUpdate, ChangeMerge:
			changed = true
		case ChangeEditedManaged, ChangeEditedRemoval, ChangeConflict:
			changed = true
			if !request.ReplaceConflicts {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("%s is %s; authorize a recoverable backup and replacement", name, change.State))
			}
		}
	}
	for name, previousHash := range priorHashes {
		if _, stillDesired := desired[name]; stillDesired {
			continue
		}
		filename := filepath.Join(target, filepath.FromSlash(name))
		if blocker := destinationSymlinkBlocker(target, name); blocker != "" {
			plan.Blockers = append(plan.Blockers, blocker)
			changed = true
			continue
		}
		contents, readErr := os.ReadFile(filename)
		if os.IsNotExist(readErr) {
			continue
		}
		change := PlannedChange{Path: name, BeforeHash: hashBytes(contents)}
		if readErr != nil || change.BeforeHash != previousHash {
			change.State = ChangeEditedRemoval
			if !request.ReplaceConflicts {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("%s is %s; authorize a recoverable backup and removal", name, change.State))
			}
		} else {
			change.State = ChangeManagedRemove
		}
		plan.Changes = append(plan.Changes, change)
		changed = true
	}
	sort.Slice(plan.Changes, func(i, j int) bool { return plan.Changes[i].Path < plan.Changes[j].Path })
	plan.NoOp = !changed && len(plan.Blockers) == 0
	return plan, nil
}

func (service *Service) Apply(ctx context.Context, plan Plan) (Result, error) {
	if len(plan.Blockers) != 0 {
		return Result{Outcome: OutcomeBlocked, NextAction: strings.Join(plan.Blockers, "; ")}, nil
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	createdTarget := false
	if _, err := os.Stat(plan.TargetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(plan.TargetPath, 0o755); err != nil {
			return Result{}, err
		}
		createdTarget = true
		plan.targetIdentity, _ = os.Stat(plan.TargetPath)
	}
	root, err := openBoundRoot(plan.TargetPath, plan.targetIdentity)
	if err != nil {
		if createdTarget {
			_ = removeBoundTarget(plan.TargetPath, plan.targetIdentity)
		}
		return Result{Outcome: OutcomeBlocked}, err
	}
	createdGit := false
	if plan.NeedGitInit {
		gitOwned, output, initErr := initializeGitAnchored(ctx, root)
		if initErr != nil {
			var cleanupErr error
			if gitOwned {
				cleanupErr = root.RemoveAll(".git")
			}
			closeErr := root.Close()
			if createdTarget {
				cleanupErr = errors.Join(cleanupErr, removeBoundTarget(plan.TargetPath, plan.targetIdentity))
			}
			return Result{Outcome: OutcomeBlocked}, errors.Join(fmt.Errorf("initialize Git: %w: %s", initErr, strings.TrimSpace(string(output))), cleanupErr, closeErr)
		}
		createdGit = gitOwned
	}
	defer root.Close()
	if plan.NoOp {
		result, err := service.finalizeVerification(ctx, plan.TargetPath, root, plan, nil)
		result.NoOp = true
		return result, err
	}

	operationID := operationIdentity(plan.desired)
	stageRoot := filepath.ToSlash(filepath.Join(".skillet", "staging", operationID))
	rollbackRoot := filepath.ToSlash(filepath.Join(stageRoot, "rollback"))
	if _, statErr := root.Lstat(filepath.FromSlash(stageRoot)); statErr == nil {
		return Result{Outcome: OutcomeBlocked}, fmt.Errorf("refusing pre-existing setup staging path %s", stageRoot)
	} else if !os.IsNotExist(statErr) {
		return Result{Outcome: OutcomeBlocked}, statErr
	}
	if err := root.MkdirAll(filepath.FromSlash(stageRoot), 0o755); err != nil {
		return Result{}, err
	}
	type promotedFile struct {
		relative string
		hadPrior bool
		removed  bool
	}
	var promoted []promotedFile
	var backups []string
	rollback := func() error {
		var rollbackErrors []error
		restoreFailed := false
		record := func(action string, err error) {
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("%s: %w", action, err))
			}
		}
		for i := len(promoted) - 1; i >= 0; i-- {
			item := promoted[i]
			destination := filepath.FromSlash(item.relative)
			if !item.removed {
				record("remove promoted "+item.relative, root.Remove(destination))
			}
			if item.hadPrior {
				prior := filepath.Join(filepath.FromSlash(rollbackRoot), filepath.FromSlash(item.relative))
				record("create restore parent "+item.relative, root.MkdirAll(filepath.Dir(destination), 0o755))
				if restoreErr := renameRoot(root, prior, destination); restoreErr != nil {
					restoreFailed = true
					record("restore prior "+item.relative, restoreErr)
				}
			}
		}
		record("remove staging", root.RemoveAll(filepath.FromSlash(stageRoot)))
		if !restoreFailed {
			for _, backup := range backups {
				record("remove rollback backup "+backup, root.Remove(filepath.FromSlash(backup)))
			}
		}
		if createdGit {
			record("remove initialized Git repository", root.RemoveAll(".git"))
		}
		if createdTarget {
			for _, createdPath := range []string{".agents", ".claude", ".skillet", ".git", "AGENTS.md", "CLAUDE.md", ".gitignore"} {
				record("remove created "+createdPath, root.RemoveAll(createdPath))
			}
			record("remove created target", removeBoundTarget(plan.TargetPath, plan.targetIdentity))
		} else {
			_ = root.Remove(filepath.Join(".skillet", "staging"))
		}
		return errors.Join(rollbackErrors...)
	}
	fail := func(cause error) (Result, error) {
		rollbackErr := rollback()
		if rollbackErr != nil {
			return Result{Outcome: OutcomeBlocked, Backups: append([]string(nil), backups...), NextAction: "Rollback was incomplete; inspect the reported paths and retained backups before retrying"}, errors.Join(cause, fmt.Errorf("rollback incomplete: %w", rollbackErr))
		}
		return Result{Outcome: OutcomeBlocked}, cause
	}

	for _, change := range plan.Changes {
		if change.State == ChangeExactAdoption || change.State == ChangeManagedUnchanged || change.State == ChangeManagedRemove || change.State == ChangeEditedRemoval {
			continue
		}
		contents := plan.desired[change.Path]
		staged := filepath.Join(filepath.FromSlash(stageRoot), "desired", filepath.FromSlash(change.Path))
		mode := plan.desiredModes[change.Path]
		if mode == 0 {
			mode = 0o644
		}
		if err := writeRootFile(root, staged, contents, mode); err != nil {
			return fail(err)
		}
	}

	for _, change := range plan.Changes {
		if change.State == ChangeExactAdoption || change.State == ChangeManagedUnchanged {
			continue
		}
		if blocker := destinationSymlinkBlocker(plan.TargetPath, change.Path); blocker != "" {
			return fail(fmt.Errorf("destination safety changed after planning: %s", blocker))
		}
		destination := filepath.FromSlash(change.Path)
		if service.hooks.BeforePromote != nil {
			if err := service.hooks.BeforePromote(change.Path); err != nil {
				return fail(fmt.Errorf("promote %s: %w", change.Path, err))
			}
		}
		_, priorStatErr := root.Lstat(destination)
		hadPrior := priorStatErr == nil
		if priorStatErr != nil && !os.IsNotExist(priorStatErr) {
			return fail(fmt.Errorf("inspect prior %s: %w", change.Path, priorStatErr))
		}
		if hadPrior {
			prior := filepath.Join(filepath.FromSlash(rollbackRoot), filepath.FromSlash(change.Path))
			if err := root.MkdirAll(filepath.Dir(prior), 0o755); err != nil {
				return fail(err)
			}
			if err := renameRoot(root, destination, prior); err != nil {
				return fail(err)
			}
			if change.State == ChangeConflict || change.State == ChangeEditedManaged || change.State == ChangeManagedRemove || change.State == ChangeEditedRemoval || change.State == ChangeMerge {
				backupRelative := filepath.ToSlash(filepath.Join(".skillet", "backups", operationID, filepath.FromSlash(change.Path)))
				backupRelative = uniqueRootPath(root, backupRelative)
				priorContents, err := root.ReadFile(prior)
				priorMode := os.FileMode(0o644)
				if info, statErr := root.Stat(prior); statErr == nil {
					priorMode = info.Mode().Perm()
				}
				if err != nil || writeRootFile(root, filepath.FromSlash(backupRelative), priorContents, priorMode) != nil {
					return fail(fmt.Errorf("create backup for %s", change.Path))
				}
				backups = append(backups, backupRelative)
			}
		}
		if change.State == ChangeManagedRemove || change.State == ChangeEditedRemoval {
			promoted = append(promoted, promotedFile{relative: change.Path, hadPrior: hadPrior, removed: true})
			continue
		}
		if err := root.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fail(err)
		}
		staged := filepath.Join(filepath.FromSlash(stageRoot), "desired", filepath.FromSlash(change.Path))
		if err := renameRoot(root, staged, destination); err != nil {
			return fail(err)
		}
		promoted = append(promoted, promotedFile{relative: change.Path, hadPrior: hadPrior})
	}
	if service.hooks.ExternalAction != nil {
		undo, repairAction, externalErr := service.hooks.ExternalAction(ctx, plan.TargetPath)
		if externalErr != nil {
			undoErr := error(nil)
			if undo != nil {
				undoErr = undo()
			}
			rollbackErr := rollback()
			if undoErr != nil {
				return Result{Outcome: OutcomePartial, NextAction: repairAction}, rollbackErr
			}
			if rollbackErr != nil {
				return Result{Outcome: OutcomeBlocked, NextAction: "Rollback was incomplete; inspect the reported paths before retrying"}, rollbackErr
			}
			return Result{Outcome: OutcomeBlocked, NextAction: externalErr.Error()}, nil
		}
	}
	result, err := service.finalizeVerification(ctx, plan.TargetPath, root, plan, backups)
	if err != nil || result.Outcome == OutcomeBlocked {
		if result.Outcome == "" {
			result.Outcome = OutcomeBlocked
			result.NextAction = "Final receipt write failed; all Managed changes were rolled back"
		}
		if rollbackErr := rollback(); rollbackErr != nil {
			result.NextAction = strings.TrimSpace(result.NextAction + "; rollback was incomplete: " + rollbackErr.Error())
			return result, rollbackErr
		}
		return result, err
	}
	_ = root.RemoveAll(filepath.FromSlash(stageRoot))
	result.Backups = backups
	return result, err
}

func (service *Service) finalizeVerification(ctx context.Context, target string, root *os.Root, plan Plan, backups []string) (Result, error) {
	const portableReceiptPath = ".skillet/workspace.json"
	const localReceiptPath = ".skillet/workspace.local.json"
	var portableSnapshot, localSnapshot rootFileSnapshot
	if plan.NoOp {
		var err error
		portableSnapshot, err = snapshotRootFile(root, portableReceiptPath)
		if err != nil {
			return Result{Outcome: OutcomeBlocked}, err
		}
		localSnapshot, err = snapshotRootFile(root, localReceiptPath)
		if err != nil {
			return Result{Outcome: OutcomeBlocked}, err
		}
	}
	rollbackNoOpReceipts := func(cause error) (Result, error) {
		if !plan.NoOp {
			return Result{}, cause
		}
		restoreErr := errors.Join(
			restoreRootSnapshot(root, portableReceiptPath, portableSnapshot),
			restoreRootSnapshot(root, localReceiptPath, localSnapshot),
		)
		if restoreErr != nil {
			return Result{Outcome: OutcomeBlocked, NextAction: "No-op verification failed and the receipt pair could not be fully restored; inspect both workspace receipts before retrying"}, errors.Join(cause, fmt.Errorf("restore receipt pair: %w", restoreErr))
		}
		return Result{Outcome: OutcomeBlocked}, cause
	}
	receipt, err := loadReceiptFromRoot(root)
	if err != nil {
		return Result{Outcome: OutcomeBlocked}, err
	}
	if err := validateBoundTarget(root, target); err != nil {
		return Result{Outcome: OutcomeBlocked}, err
	}
	toolResults := service.prober.Probe(ctx, target, root, receipt)
	if err := validateBoundTarget(root, target); err != nil {
		return Result{Outcome: OutcomeBlocked}, err
	}
	staticFailures := make([]string, 0)
	for _, toolResult := range toolResults {
		if !toolResult.StaticVerified {
			staticFailures = append(staticFailures, toolResult.Tool+": "+toolResult.Reason)
		}
	}
	if len(staticFailures) != 0 {
		return Result{Outcome: OutcomeBlocked, NextAction: "Static verification failed: " + strings.Join(staticFailures, "; ")}, nil
	}
	outcome := OutcomeVerified
	nextActions := make([]string, 0)
	for _, toolResult := range toolResults {
		if !toolResult.StaticVerified || !toolResult.RuntimeVerified {
			outcome = OutcomeConfiguredUnverified
		}
		if toolResult.NextAction != "" {
			nextActions = append(nextActions, toolResult.NextAction)
		}
	}
	if len(toolResults) != 2 {
		outcome = OutcomeConfiguredUnverified
	}
	receipt.Outcome = outcome
	if portableChanges := portableReceiptChanges(plan.Changes); len(portableChanges) != 0 {
		receipt.Decisions = portableChanges
		receipt.Backups = append([]string(nil), backups...)
	}
	local := LocalReceipt{SchemaVersion: 1, AbsoluteTarget: target, Outcome: outcome, ToolResults: toolResults}
	receipt.LocalReceiptSHA256 = ""
	local.PortableReceiptSHA256 = portableIdentityHash(receipt)
	local.SelfSHA256 = localIdentityHash(local)
	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := writeRootIfChanged(root, portableReceiptPath, append(receiptBytes, '\n'), 0o644); err != nil {
		return rollbackNoOpReceipts(err)
	}
	localBytes, err := json.MarshalIndent(local, "", "  ")
	if err != nil {
		return rollbackNoOpReceipts(err)
	}
	if err := writeRootIfChanged(root, localReceiptPath, append(localBytes, '\n'), 0o644); err != nil {
		return rollbackNoOpReceipts(err)
	}
	return Result{
		Outcome: outcome, ReceiptPath: filepath.Join(target, ".skillet", "workspace.json"),
		LocalReceiptPath: filepath.Join(target, ".skillet", "workspace.local.json"),
		NextAction:       strings.Join(nextActions, "; "),
	}, nil
}

func portableReceiptChanges(changes []PlannedChange) []PlannedChange {
	filtered := make([]PlannedChange, 0, len(changes))
	for _, change := range changes {
		if change.Path == ".skillet/workspace.local.json" || change.State == ChangeManagedUnchanged || change.State == ChangeExactAdoption {
			continue
		}
		filtered = append(filtered, change)
	}
	return filtered
}

type rootFileSnapshot struct {
	contents []byte
	mode     os.FileMode
	exists   bool
}

func snapshotRootFile(root *os.Root, name string) (rootFileSnapshot, error) {
	contents, err := root.ReadFile(name)
	if os.IsNotExist(err) {
		return rootFileSnapshot{}, nil
	}
	if err != nil {
		return rootFileSnapshot{}, err
	}
	info, err := root.Stat(name)
	if err != nil {
		return rootFileSnapshot{}, err
	}
	return rootFileSnapshot{contents: contents, mode: info.Mode().Perm(), exists: true}, nil
}

func restoreRootSnapshot(root *os.Root, name string, snapshot rootFileSnapshot) error {
	if err := root.RemoveAll(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !snapshot.exists {
		return nil
	}
	return writeRootFile(root, name, snapshot.contents, snapshot.mode)
}

func normalizeTarget(input string) (string, bool, error) {
	if strings.TrimSpace(input) == "" {
		return "", false, fmt.Errorf("setup target path is required")
	}
	target, err := filepath.Abs(filepath.Clean(input))
	if err != nil {
		return "", false, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(target); resolveErr == nil {
		target = resolved
	}
	home, _ := os.UserHomeDir()
	if target == filepath.VolumeName(target)+string(filepath.Separator) || (home != "" && sameFilePath(target, home)) {
		return "", false, fmt.Errorf("refusing unsafe setup target %q", target)
	}
	if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
		return "", false, fmt.Errorf("setup target is not a directory: %s", target)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", false, statErr
	}
	if ancestor := enclosingGitRoot(target); ancestor != "" && !sameFilePath(ancestor, target) {
		return "", false, fmt.Errorf("setup target %s is nested inside repository %s; select the repository root", target, ancestor)
	}
	_, gitErr := os.Stat(filepath.Join(target, ".git"))
	return target, os.IsNotExist(gitErr), nil
}

func enclosingGitRoot(target string) string {
	start := target
	if _, err := os.Stat(start); os.IsNotExist(err) {
		start = filepath.Dir(start)
	}
	for directory := start; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
			return directory
		}
		if sameFilePath(directory, filepath.Dir(directory)) {
			return ""
		}
	}
}

func classifyChange(target, relative string, desired []byte, desiredMode os.FileMode, prior map[string]string, priorModes map[string]uint32) PlannedChange {
	if desiredMode == 0 {
		desiredMode = 0o644
	}
	change := PlannedChange{Path: relative, DesiredHash: hashBytes(desired), DesiredMode: uint32(desiredMode.Perm())}
	actual, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(relative)))
	if os.IsNotExist(err) {
		change.State = ChangeAbsent
		return change
	}
	if err != nil {
		change.State = ChangeConflict
		return change
	}
	change.BeforeHash = hashBytes(actual)
	if info, statErr := os.Stat(filepath.Join(target, filepath.FromSlash(relative))); statErr == nil {
		change.BeforeMode = uint32(info.Mode().Perm())
	}
	if change.BeforeHash == change.DesiredHash {
		if _, managed := prior[relative]; managed {
			if expectedMode := priorModes[relative]; expectedMode != 0 && change.BeforeMode != expectedMode {
				change.State = ChangeEditedManaged
				return change
			}
			if change.BeforeMode != change.DesiredMode {
				change.State = ChangeManagedUpdate
				return change
			}
			change.State = ChangeManagedUnchanged
		} else {
			if change.BeforeMode != change.DesiredMode {
				change.State = ChangeConflict
				return change
			}
			change.State = ChangeExactAdoption
		}
		return change
	}
	if previousHash, managed := prior[relative]; managed {
		if previousHash == change.BeforeHash {
			change.State = ChangeManagedUpdate
		} else {
			change.State = ChangeEditedManaged
		}
		return change
	}
	if relative == ".gitignore" && strings.HasSuffix(string(desired), ".skillet/workspace.local.json\n") {
		change.State = ChangeMerge
		return change
	}
	change.State = ChangeConflict
	return change
}

func loadPriorReceipt(target string) (WorkspaceReceipt, error) {
	contents, err := os.ReadFile(filepath.Join(target, ".skillet", "workspace.json"))
	if os.IsNotExist(err) {
		return WorkspaceReceipt{}, nil
	}
	if err != nil {
		return WorkspaceReceipt{}, fmt.Errorf("read prior workspace receipt: %w", err)
	}
	var receipt WorkspaceReceipt
	if err := json.Unmarshal(contents, &receipt); err != nil {
		return WorkspaceReceipt{}, fmt.Errorf("parse prior workspace receipt: %w", err)
	}
	return receipt, nil
}

func loadLocalReceipt(target string) (LocalReceipt, error) {
	contents, err := os.ReadFile(filepath.Join(target, ".skillet", "workspace.local.json"))
	if os.IsNotExist(err) {
		return LocalReceipt{}, nil
	}
	if err != nil {
		return LocalReceipt{}, fmt.Errorf("read prior local workspace receipt: %w", err)
	}
	var receipt LocalReceipt
	if err := json.Unmarshal(contents, &receipt); err != nil {
		return LocalReceipt{}, fmt.Errorf("parse prior local workspace receipt: %w", err)
	}
	if receipt.SchemaVersion != 1 {
		return LocalReceipt{}, fmt.Errorf("prior local workspace receipt schema %d is unsupported", receipt.SchemaVersion)
	}
	return receipt, nil
}

func validateReceiptPair(target string, portable WorkspaceReceipt, local LocalReceipt) error {
	if portable.SchemaVersion == 0 && local.SchemaVersion == 0 {
		return nil
	}
	if portable.SchemaVersion != 0 && local.SchemaVersion == 0 {
		tracked := exec.Command("git", "-C", target, "ls-files", "--error-unmatch", "--", ".skillet/workspace.json")
		if output, err := tracked.CombinedOutput(); err != nil {
			return fmt.Errorf("local workspace receipt is absent and the portable receipt is not tracked by Git: %w: %s", err, strings.TrimSpace(string(output)))
		}
		clean := exec.Command("git", "-C", target, "diff", "--quiet", "HEAD", "--", ".skillet/workspace.json")
		if err := clean.Run(); err != nil {
			return fmt.Errorf("local workspace receipt is absent and the portable receipt differs from committed Git state")
		}
		return nil
	}
	if local.SelfSHA256 == "" && portable.LocalReceiptSHA256 == "" && local.PortableReceiptSHA256 == "" {
		return nil // schema-1 migration from receipts created before cross-identities
	}
	if local.SelfSHA256 == "" && portable.LocalReceiptSHA256 != "" && local.PortableReceiptSHA256 != "" {
		if actual := legacyLocalIdentityHash(local); actual != portable.LocalReceiptSHA256 {
			return fmt.Errorf("local workspace receipt was edited: identity %s != %s", actual, portable.LocalReceiptSHA256)
		}
		if actual := portableIdentityHash(portable); actual != local.PortableReceiptSHA256 {
			return fmt.Errorf("portable workspace receipt was edited: identity %s != %s", actual, local.PortableReceiptSHA256)
		}
		return nil
	}
	if local.PortableReceiptSHA256 == "" || local.SelfSHA256 == "" {
		return fmt.Errorf("workspace receipt identity pair is incomplete")
	}
	if actual := portableIdentityHash(portable); actual != local.PortableReceiptSHA256 {
		return fmt.Errorf("portable workspace receipt was edited: identity %s != %s", actual, local.PortableReceiptSHA256)
	}
	if actual := localIdentityHash(local); actual != local.SelfSHA256 {
		return fmt.Errorf("local workspace receipt was edited: identity %s != %s", actual, local.SelfSHA256)
	}
	return nil
}

func portableIdentityHash(receipt WorkspaceReceipt) string {
	receipt.LocalReceiptSHA256 = ""
	contents, _ := json.Marshal(receipt)
	return hashBytes(contents)
}

func localIdentityHash(receipt LocalReceipt) string {
	receipt.SelfSHA256 = ""
	contents, _ := json.Marshal(receipt)
	return hashBytes(contents)
}

func legacyLocalIdentityHash(receipt LocalReceipt) string {
	receipt.PortableReceiptSHA256 = ""
	receipt.SelfSHA256 = ""
	contents, _ := json.Marshal(receipt)
	return hashBytes(contents)
}

func snapshotReceipt(receipt WorkspaceReceipt) ReceiptTransition {
	transition := ReceiptTransition{
		CatalogVersion: receipt.CatalogVersion, BundleIDs: append([]string(nil), receipt.BundleIDs...), Outcome: receipt.Outcome,
		Decisions: append([]PlannedChange(nil), receipt.Decisions...), Backups: append([]string(nil), receipt.Backups...),
	}
	for _, member := range receipt.Members {
		transition.Members = append(transition.Members, ReceiptMemberIdentity{
			Name: member.Name, ReviewedRevision: member.ReviewedRevision, ResolvedRevision: member.ResolvedRevision,
			ResolvedSHA256: member.ResolvedSHA256, Activation: member.Activation, Drift: member.Drift, DriftAccepted: member.DriftAccepted,
		})
	}
	return transition
}

func receiptConfigurationEqual(left, right WorkspaceReceipt) bool {
	leftSnapshot, _ := json.Marshal(snapshotReceipt(WorkspaceReceipt{CatalogVersion: left.CatalogVersion, BundleIDs: left.BundleIDs, Members: left.Members}))
	rightSnapshot, _ := json.Marshal(snapshotReceipt(WorkspaceReceipt{CatalogVersion: right.CatalogVersion, BundleIDs: right.BundleIDs, Members: right.Members}))
	return string(leftSnapshot) == string(rightSnapshot)
}

func validatePriorReceipt(receipt WorkspaceReceipt) error {
	if receipt.SchemaVersion == 0 {
		if receipt.CatalogVersion == "" && len(receipt.ManagedPaths) == 0 && len(receipt.Members) == 0 {
			return nil
		}
		return fmt.Errorf("prior workspace receipt schema is missing")
	}
	if receipt.SchemaVersion != 1 {
		return fmt.Errorf("prior workspace receipt schema %d is unsupported", receipt.SchemaVersion)
	}
	for _, managed := range receipt.ManagedPaths {
		if !validManagedRelativePath(managed.Path) {
			return fmt.Errorf("prior workspace receipt contains unsafe managed path %q", managed.Path)
		}
	}
	return nil
}

func validManagedRelativePath(name string) bool {
	if name == "" || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return false
	}
	for _, exact := range []string{"AGENTS.md", "CLAUDE.md", ".gitignore", ".skillet/workspace.json", ".skillet/workspace.local.json"} {
		if name == exact {
			return true
		}
	}
	for _, prefix := range []string{".skillet/managed/skills/", ".claude/skills/", ".agents/skills/"} {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return true
		}
	}
	return false
}

func destinationSymlinkBlocker(target, relative string) string {
	if !validManagedRelativePath(relative) {
		return fmt.Sprintf("refusing unmanaged destination path %q", relative)
	}
	current := target
	parts := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Sprintf("inspect managed destination %s: %v", filepath.ToSlash(relative), err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Sprintf("managed destination %s crosses symlink %s", filepath.ToSlash(relative), current)
		}
		if !info.IsDir() {
			return fmt.Sprintf("managed destination %s has non-directory ancestor %s", filepath.ToSlash(relative), current)
		}
	}
	final := filepath.Join(target, filepath.FromSlash(relative))
	if info, err := os.Lstat(final); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Sprintf("managed destination %s is a symlink", filepath.ToSlash(relative))
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Sprintf("inspect managed destination %s: %v", filepath.ToSlash(relative), err)
	}
	return ""
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func mergeIgnore(existing []byte, line string) []byte {
	text := string(existing)
	for _, current := range strings.Split(text, "\n") {
		if strings.TrimSpace(current) == line {
			return existing
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text + line + "\n")
}

func operationIdentity(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	combined := make(map[string][]byte, len(files))
	for _, name := range names {
		combined[name] = files[name]
	}
	return hashFiles(combined)[:12]
}

func writeFile(filename string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, contents, mode)
}

func initializeGitAnchored(_ context.Context, root *os.Root) (bool, []byte, error) {
	if err := root.Mkdir(".git", 0o755); err != nil {
		return false, nil, fmt.Errorf("claim .git for managed initialization: %w", err)
	}
	for _, directory := range []string{
		".git/objects/info", ".git/objects/pack", ".git/refs/heads", ".git/refs/tags", ".git/info",
	} {
		if err := root.MkdirAll(directory, 0o755); err != nil {
			return true, nil, err
		}
	}
	files := map[string]string{
		".git/HEAD":         "ref: refs/heads/main\n",
		".git/config":       "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n\tlogallrefupdates = true\n",
		".git/description":  "Unnamed repository; managed workspace initialized by Skillet.\n",
		".git/info/exclude": "# Managed workspace repository exclusions\n",
	}
	for name, contents := range files {
		if err := writeRootFile(root, name, []byte(contents), 0o644); err != nil {
			return true, nil, err
		}
	}
	return true, nil, nil
}

func writeRootFile(root *os.Root, name string, contents []byte, mode os.FileMode) error {
	if err := root.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(contents)
	chmodErr := file.Chmod(mode)
	closeErr := file.Close()
	return errors.Join(writeErr, chmodErr, closeErr)
}

func writeRootIfChanged(root *os.Root, name string, contents []byte, mode os.FileMode) error {
	if current, err := root.ReadFile(name); err == nil && string(current) == string(contents) {
		return nil
	}
	temp := name + ".skillet-write-" + hashBytes(contents)[:12]
	if err := writeRootFile(root, temp, contents, mode); err != nil {
		return err
	}
	if err := renameRoot(root, temp, name); err != nil {
		_ = root.Remove(temp)
		return err
	}
	return nil
}

func openBoundRoot(target string, expected os.FileInfo) (*os.Root, error) {
	lstat, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	if lstat.Mode()&os.ModeSymlink != 0 || !lstat.IsDir() {
		return nil, fmt.Errorf("setup target identity changed or is a symlink: %s", target)
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	current, err := os.Stat(target)
	if err != nil || !os.SameFile(opened, current) || (expected != nil && !os.SameFile(opened, expected)) {
		root.Close()
		return nil, fmt.Errorf("setup target identity changed after planning: %s", target)
	}
	return root, nil
}

func validateBoundTarget(root *os.Root, target string) error {
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Lstat(target)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return fmt.Errorf("setup target identity changed during verification: %s", target)
	}
	return nil
}

func removeBoundTarget(target string, expected os.FileInfo) error {
	current, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || expected == nil || !os.SameFile(current, expected) {
		return fmt.Errorf("refusing to remove changed target identity %s", target)
	}
	return os.Remove(target)
}

func uniqueRootPath(root *os.Root, base string) string {
	if _, err := root.Lstat(filepath.FromSlash(base)); os.IsNotExist(err) {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s.%d", base, index)
		if _, err := root.Lstat(filepath.FromSlash(candidate)); os.IsNotExist(err) {
			return candidate
		}
	}
}

func loadReceiptFromRoot(root *os.Root) (WorkspaceReceipt, error) {
	contents, err := root.ReadFile(filepath.Join(".skillet", "workspace.json"))
	if err != nil {
		return WorkspaceReceipt{}, fmt.Errorf("read workspace receipt during verification: %w", err)
	}
	var receipt WorkspaceReceipt
	if err := json.Unmarshal(contents, &receipt); err != nil {
		return WorkspaceReceipt{}, fmt.Errorf("parse workspace receipt during verification: %w", err)
	}
	return receipt, nil
}

func fileExists(filename string) bool {
	_, err := os.Lstat(filename)
	return err == nil
}

func uniquePath(base string) string {
	if !fileExists(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !fileExists(candidate) {
			return candidate
		}
	}
}

func sameFilePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func removeEmptyParents(stop, start string) {
	for directory := start; !sameFilePath(directory, stop); directory = filepath.Dir(directory) {
		if err := os.Remove(directory); err != nil {
			return
		}
	}
}
