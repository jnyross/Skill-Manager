package engine

// Write-ownership records — Skillet's answer to "did I write this, or did the
// user?" for the two files it edits that have no other provenance: Codex's
// <CodexHome>/config.toml and a Codex skill's <skill>/agents/openai.yaml.
//
// Both files can legitimately contain content Skillet never wrote: a
// config.toml `[[skills.config]]` block a human (or Codex itself) authored, or
// an openai.yaml that existed before Skillet ever touched the skill. Without a
// record, Unsuppress and "unset Manual-only" had to guess, and both guessed in
// the destructive direction — removing whatever matched, and deleting the file
// outright once nothing was left in it. These records make the reverse
// operation remove only what Skillet added, and delete a file only when
// Skillet created it.
//
// Storage mirrors the existing suppression records (suppress.go): one small
// JSON file per skill under Skillet's own data directory. Codex config records
// live in the <DataDir>/suppressed/codex/ subdirectory — loadSuppressionRecords
// skips subdirectories, so Plugin suppression records are unaffected — and
// policy-file records live in <DataDir>/manual-only/. Both are keyed by the
// skill's absolute SKILL.md / skill-folder path, which is what identifies a
// Codex skill in config.toml in the first place.

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"
)

// codexConfigRecord records the exact `[[skills.config]]` block Skillet
// appended to config.toml for one skill, and whether Skillet created
// config.toml in the process.
type codexConfigRecord struct {
	SkillName     string    `json:"skillName"`
	SkillMDPath   string    `json:"skillMdPath"`
	Block         string    `json:"block"`
	CreatedConfig bool      `json:"createdConfig"`
	SuppressedAt  time.Time `json:"suppressedAt"`
}

// codexPolicyRecord records that Skillet created a Codex skill's
// agents/openai.yaml from nothing, so unsetting Manual-only may delete it
// again — and, crucially, must not delete one the user already had.
type codexPolicyRecord struct {
	SkillName   string    `json:"skillName"`
	PolicyPath  string    `json:"policyPath"`
	CreatedFile bool      `json:"createdFile"`
	AppliedAt   time.Time `json:"appliedAt"`
}

func codexConfigRecordDir(dataDir string) string {
	return filepath.Join(dataDir, "suppressed", "codex")
}

func codexPolicyRecordDir(dataDir string) string {
	return filepath.Join(dataDir, "manual-only")
}

// pathRecordID is a deterministic, filesystem-safe, collision-resistant id for
// a skill path: a readable name plus a hash of the full path, so two skills
// with the same folder name in different roots never share a record.
func pathRecordID(name, path string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(absolutePath(path)))
	return sanitizeIDPart(name) + "-" + fmt.Sprintf("%016x", h.Sum64())
}

func readJSONRecord(path string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read record %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		// A corrupt record is treated as absent rather than fatal: the
		// conservative branch (leave the user's file alone) is the safe one.
		return false, nil
	}
	return true, nil
}

func writeJSONRecord(dir, path string, value any) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create record directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

// removeJSONRecord deletes a record and then prunes the directories it lived
// in if they are now empty (os.Remove refuses a non-empty directory), so a
// suppress-then-unsuppress round trip leaves Skillet's data directory exactly
// as it found it — the same tree-identical guarantee the Codex config round
// trip itself makes.
func removeJSONRecord(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove record: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.Remove(dir); err == nil && filepath.Base(dir) == "codex" {
		_ = os.Remove(filepath.Dir(dir))
	}
	return nil
}

func codexConfigRecordPath(dataDir, skillName, skillMDPath string) string {
	return filepath.Join(codexConfigRecordDir(dataDir), pathRecordID(skillName, skillMDPath)+".json")
}

func loadCodexConfigRecord(dataDir, skillName, skillMDPath string) (codexConfigRecord, bool, error) {
	var record codexConfigRecord
	found, err := readJSONRecord(codexConfigRecordPath(dataDir, skillName, skillMDPath), &record)
	return record, found, err
}

func saveCodexConfigRecord(dataDir string, record codexConfigRecord) error {
	return writeJSONRecord(codexConfigRecordDir(dataDir), codexConfigRecordPath(dataDir, record.SkillName, record.SkillMDPath), record)
}

func deleteCodexConfigRecord(dataDir, skillName, skillMDPath string) error {
	return removeJSONRecord(codexConfigRecordPath(dataDir, skillName, skillMDPath))
}

func codexPolicyRecordPath(dataDir, skillName, policyPath string) string {
	return filepath.Join(codexPolicyRecordDir(dataDir), pathRecordID(skillName, policyPath)+".json")
}

func loadCodexPolicyRecord(dataDir, skillName, policyPath string) (codexPolicyRecord, bool, error) {
	var record codexPolicyRecord
	found, err := readJSONRecord(codexPolicyRecordPath(dataDir, skillName, policyPath), &record)
	return record, found, err
}

func saveCodexPolicyRecord(dataDir string, record codexPolicyRecord) error {
	return writeJSONRecord(codexPolicyRecordDir(dataDir), codexPolicyRecordPath(dataDir, record.SkillName, record.PolicyPath), record)
}

func deleteCodexPolicyRecord(dataDir, skillName, policyPath string) error {
	return removeJSONRecord(codexPolicyRecordPath(dataDir, skillName, policyPath))
}
