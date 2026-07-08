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
	ActivationSuppressed ActivationState = "Suppressed"
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

// SuppressionRecord is Skillet-owned state recording that a Plugin skill has
// been Suppressed (CONTEXT.md: hidden from the model and slash menu while its
// plugin stays installed and intact). It is keyed by marketplace + plugin +
// skill name — never by a specific cache/version directory — because plugin
// cache directories are versioned and replaced whole on update (see
// installed_plugins.json's installPath, which changes per version). Keying
// this way is what lets the self-healing loop (internal/engine/suppress.go)
// find and re-apply the suppression to a new version directory after a
// plugin update.
type SuppressionRecord struct {
	Marketplace  string    `json:"marketplace"`
	Plugin       string    `json:"plugin"`
	SkillName    string    `json:"skillName"`
	SuppressedAt time.Time `json:"suppressedAt"`
}

// RemovedConfigEntry records a stale Codex config.toml `[[skills.config]]`
// block removed on Uninstall, so Restore can splice it back byte-identically.
// Offset is the byte position within the config file (as it stands after
// removal) where Raw must be reinserted.
type RemovedConfigEntry struct {
	Offset int    `json:"offset"`
	Raw    string `json:"raw"`
}
