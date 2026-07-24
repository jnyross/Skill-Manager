package engine

import "time"

type Source string

const (
	SourcePersonal Source = "Personal"
	SourcePlugin   Source = "Plugin"
	SourceCodex    Source = "Codex"
	SourceProject  Source = "Project"
)

type Tool string

const (
	ToolClaudeCode Tool = "Claude Code"
	ToolCodex      Tool = "Codex"
)

type ActivationState string

const (
	ActivationAuto       ActivationState = "Auto"
	ActivationManualOnly ActivationState = "Manual-only"
	// ActivationDisabled is Codex's own native config.toml `[[skills.config]]
	// enabled = false` disable (see docs/research/skill-mechanisms.md,
	// "Settings-level disable exists"). Codex Suppress (internal/engine/
	// codex_suppress.go) writes exactly this native entry, so a
	// Skillet-initiated Codex Suppress and a config.toml entry a user (or
	// another tool) wrote by hand are mechanistically and functionally
	// identical — Skillet has no way to tell them apart from the file alone,
	// and it would be misleading to claim it could by labeling one
	// "Suppressed" and the other "Disabled". Both render as Disabled.
	ActivationDisabled ActivationState = "Disabled"
	// ActivationSuppressed is Skillet-owned Plugin Suppress only (suppress.go)
	// — a genuinely Skillet-only concept with no native Claude Code
	// equivalent, requiring a self-healing record Skillet alone maintains.
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
	Tool        Tool
	Kind        Kind
	Location    string
	Activation  ActivationState
	Plugin      *PluginInfo
	// DeclaredManualOnlyForClaude is true when a Codex-scanned SKILL.md
	// declares disable-model-invocation: true. In Claude Code that field
	// means manual-only, but Codex runtime activation is governed by
	// agents/openai.yaml, so the declared intent has no effect in Codex.
	// The detail pane uses this flag to surface the mismatch.
	DeclaredManualOnlyForClaude bool

	// The cost fields below are ESTIMATES (internal/engine/cost.go), never
	// exact token counts, and every surface that shows them must say so.
	//
	// DescriptionTokens is the headline number: what Auto-activation injects
	// into every session with this Skill's Tool, used or not. It is populated
	// for every Skill regardless of Activation, so the TUI can answer "what
	// would this cost if I turned it back on".
	DescriptionTokens int
	// BodyBytes and BodyTokens are the whole SKILL.md (or, for a Codex prompt,
	// the prompt file): what invoking the Skill costs, on top of the standing
	// description cost.
	BodyBytes  int64
	BodyTokens int
	// FileCount and TotalBytes cover the Skill's own directory — references,
	// scripts, and assets included. They are what the Skill occupies, not what
	// it injects; a Skill that reads its own reference files pays for them only
	// when it does.
	//
	// Unlike the fields above, these two are NOT filled in by Inventory(): they
	// are the only cost numbers that need a directory walk, and doing that for
	// every Skill on every refresh costs more than the whole rest of the scan.
	// Call MeasureSkillFiles (internal/engine/cost.go) for the Skills you are
	// about to show; until then they are zero, meaning "not measured".
	FileCount  int
	TotalBytes int64
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
	Tool                 Tool                 `json:"tool"`
	OriginRepo           string               `json:"originRepo,omitempty"`
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
// Offset is a skeleton-relative byte position (see internal/engine/
// codex_config.go buildSkeleton) where Raw must be reinserted, so it stays
// valid even when other Codex skills' config entries are archived or restored
// in between.
type RemovedConfigEntry struct {
	Offset int    `json:"offset"`
	Raw    string `json:"raw"`
}

// LibrarySourceKind selects which install-source descriptor fields are set on
// a Library entry (CONTEXT.md Library; ADR 0004). Exactly one field group is
// meaningful per kind.
type LibrarySourceKind string

const (
	LibrarySourceSkillsSh    LibrarySourceKind = "skills.sh"
	LibrarySourceGit         LibrarySourceKind = "git"
	LibrarySourceMarketplace LibrarySourceKind = "marketplace"
	LibrarySourceLocalPath   LibrarySourceKind = "local-path"
)

// LibrarySource is an install-source pointer resolved fresh on Install — not
// a snapshot of skill content (ADR 0004).
type LibrarySource struct {
	Kind LibrarySourceKind `json:"kind"`

	// Kind == LibrarySourceSkillsSh
	SkillsShRepo  string `json:"skillsShRepo,omitempty"`
	SkillsShSkill string `json:"skillsShSkill,omitempty"` // optional; empty = all skills from source

	// Kind == LibrarySourceGit
	GitURL     string `json:"gitUrl,omitempty"`
	GitRef     string `json:"gitRef,omitempty"`
	GitSubPath string `json:"gitSubPath,omitempty"`

	// Kind == LibrarySourceMarketplace
	Marketplace       string `json:"marketplace,omitempty"`
	PluginName        string `json:"pluginName,omitempty"`
	MarketplaceSource string `json:"marketplaceSource,omitempty"` // optional; for marketplace add when unknown

	// Kind == LibrarySourceLocalPath
	LocalPath string `json:"localPath,omitempty"`
}

// LibraryEntry is one item in the user's Library catalog (CONTEXT.md: spans
// Personal/Plugin/Codex — never Project as a catalog home). Tool applies to
// skill entries; marketplace/plugin entries leave Tool empty.
type LibraryEntry struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Kind    Kind          `json:"kind,omitempty"`
	Tool    Tool          `json:"tool,omitempty"`
	Source  LibrarySource `json:"source"`
	AddedAt time.Time     `json:"addedAt"`
}

// InstallTargetKind is where an Install places its result — Personal
// (user-level Claude Code / Codex skill directories) or a specific repo
// (Project-scoped). See CONTEXT.md Install.
type InstallTargetKind string

const (
	InstallTargetPersonal InstallTargetKind = "personal"
	InstallTargetProject  InstallTargetKind = "project"
)

// InstallTarget names the placement destination for InstallLibraryEntry.
// RepoRoot is set only when Kind == InstallTargetProject and must be one of
// the engine's already-resolved project roots (never free-text input).
type InstallTarget struct {
	Kind     InstallTargetKind
	RepoRoot string
}
