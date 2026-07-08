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
	ID               string    `json:"-"`
	Name             string    `json:"name"`
	Source           Source    `json:"source"`
	OriginalLocation string    `json:"originalLocation"`
	ArchivedAt       time.Time `json:"archivedAt"`
	IsSymlink        bool      `json:"isSymlink"`
	SymlinkTarget    string    `json:"symlinkTarget"`
}
