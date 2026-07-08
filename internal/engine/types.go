package engine

import "time"

type Source string

const (
	SourcePersonal Source = "Personal"
	SourcePlugin   Source = "Plugin"
	SourceCodex    Source = "Codex"
)

type ActivationState string

const (
	ActivationAuto       ActivationState = "Auto"
	ActivationManualOnly ActivationState = "Manual-only"
	ActivationDisabled   ActivationState = "Disabled"
)

type Kind string

const (
	KindSkill  Kind = "skill"
	KindPrompt Kind = "prompt"
)

type PluginInfo struct {
	Plugin      string
	Marketplace string
	SkillCount  int
}

type Skill struct {
	Name        string
	Description string
	Source      Source
	Kind        Kind
	Location    string
	Activation  ActivationState
	Plugin      *PluginInfo
}

type Notice struct {
	Message string
}

type Inventory struct {
	Skills  []Skill
	Notices []Notice
}

type ArchiveEntry struct {
	ID                   string               `json:"-"`
	Name                 string               `json:"name"`
	Source               Source               `json:"source"`
	Kind                 Kind                 `json:"kind"`
	OriginalLocation     string               `json:"originalLocation"`
	ArchivedAt           time.Time            `json:"archivedAt"`
	IsSymlink            bool                 `json:"isSymlink"`
	SymlinkTarget        string               `json:"symlinkTarget"`
	RemovedConfigEntries []RemovedConfigEntry `json:"removedConfigEntries,omitempty"`
}

// RemovedConfigEntry records a stale Codex config.toml `[[skills.config]]`
// block removed on Uninstall, so Restore can splice it back byte-identically.
// Offset is the byte position within the config file (as it stands after
// removal) where Raw must be reinserted.
type RemovedConfigEntry struct {
	Offset int    `json:"offset"`
	Raw    string `json:"raw"`
}
