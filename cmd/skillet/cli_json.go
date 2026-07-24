package main

// JSON views for the scriptable surface. Field names are derived from the
// engine types (internal/engine/types.go) and CONTEXT.md's vocabulary, and
// every document is an object carrying "schemaVersion" so later work packages
// can add fields (WP5's token and context cost, for one) without breaking a
// consumer that reads today's output.

import (
	"encoding/json"
	"io"
	"time"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

// jsonSchemaVersion changes only if an existing field is removed, renamed, or
// given a new meaning. Adding a field does not bump it.
const jsonSchemaVersion = 1

type pluginJSON struct {
	// Plugin, Marketplace and SkillCount mirror engine.PluginInfo: which
	// plugin bundles this Skill, where the plugin came from, and how many
	// Skills that plugin install carries.
	Plugin      string `json:"plugin"`
	Marketplace string `json:"marketplace"`
	SkillCount  int    `json:"skillCount"`
}

type skillJSON struct {
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	Description   string `json:"description"`
	Source        string `json:"source"`
	Tool          string `json:"tool"`
	Kind          string `json:"kind"`
	Activation    string `json:"activation"`
	Location      string `json:"location"`
	// DeclaredManualOnlyForClaude reports a Codex-scanned SKILL.md that
	// declares disable-model-invocation: true — meaningful to Claude Code,
	// inert in Codex (see engine.Skill).
	DeclaredManualOnlyForClaude bool        `json:"declaredManualOnlyForClaude"`
	Plugin                      *pluginJSON `json:"plugin,omitempty"`
	Cost                        costJSON    `json:"cost"`
}

// costJSON is one Skill's context cost. Every number in it is an ESTIMATE
// (engine.EstimateTokens, roughly four bytes per token) — a consumer that needs
// exact counts must tokenize the files itself.
//
// descriptionTokens is what Auto-activation injects into every session with the
// Skill's Tool; it is reported for every Skill regardless of Activation, so a
// caller can answer "what would turning this back on cost". bodyTokens is what
// invoking the Skill costs. fileCount and totalBytes cover the Skill's whole
// directory.
type costJSON struct {
	DescriptionTokens int   `json:"descriptionTokens"`
	BodyBytes         int64 `json:"bodyBytes"`
	BodyTokens        int   `json:"bodyTokens"`
	FileCount         int   `json:"fileCount"`
	TotalBytes        int64 `json:"totalBytes"`
}

type noticeJSON struct {
	Message string `json:"message"`
}

type archiveEntryJSON struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Source           string `json:"source"`
	Tool             string `json:"tool"`
	Kind             string `json:"kind"`
	OriginalLocation string `json:"originalLocation"`
	OriginRepo       string `json:"originRepo,omitempty"`
	ArchivedAt       string `json:"archivedAt"`
}

type listJSON struct {
	SchemaVersion int                `json:"schemaVersion"`
	Skills        []skillJSON        `json:"skills"`
	Notices       []noticeJSON       `json:"notices"`
	Archive       []archiveEntryJSON `json:"archive"`
}

type showJSON struct {
	SchemaVersion int       `json:"schemaVersion"`
	Skill         skillJSON `json:"skill"`
}

type libraryListJSON struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Entries       []engine.LibraryEntry `json:"entries"`
}

type libraryEntryJSON struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Entry         engine.LibraryEntry `json:"entry"`
}

type bundleListJSON struct {
	SchemaVersion int             `json:"schemaVersion"`
	Bundles       []engine.Bundle `json:"bundles"`
}

type installTargetJSON struct {
	Kind     string `json:"kind"`
	RepoRoot string `json:"repoRoot,omitempty"`
}

type bundleInstallJSON struct {
	SchemaVersion int               `json:"schemaVersion"`
	Bundle        engine.Bundle     `json:"bundle"`
	Target        installTargetJSON `json:"target"`
	Installed     int               `json:"installed"`
}

func newSkillJSON(skill engine.Skill) skillJSON {
	view := skillJSON{
		Name:                        skill.Name,
		QualifiedName:               qualifiedSkillName(skill),
		Description:                 skill.Description,
		Source:                      string(skill.Source),
		Tool:                        string(skill.Tool),
		Kind:                        string(skill.Kind),
		Activation:                  string(skill.Activation),
		Location:                    skill.Location,
		DeclaredManualOnlyForClaude: skill.DeclaredManualOnlyForClaude,
		Cost: costJSON{
			DescriptionTokens: skill.DescriptionTokens,
			BodyBytes:         skill.BodyBytes,
			BodyTokens:        skill.BodyTokens,
			FileCount:         skill.FileCount,
			TotalBytes:        skill.TotalBytes,
		},
	}
	if skill.Plugin != nil {
		view.Plugin = &pluginJSON{
			Plugin:      skill.Plugin.Plugin,
			Marketplace: skill.Plugin.Marketplace,
			SkillCount:  skill.Plugin.SkillCount,
		}
	}
	return view
}

func newSkillsJSON(skills []engine.Skill) []skillJSON {
	views := make([]skillJSON, 0, len(skills))
	for _, skill := range skills {
		views = append(views, newSkillJSON(skill))
	}
	return views
}

func newNoticesJSON(notices []engine.Notice) []noticeJSON {
	views := make([]noticeJSON, 0, len(notices))
	for _, notice := range notices {
		views = append(views, noticeJSON{Message: notice.Message})
	}
	return views
}

func newArchiveJSON(entries []engine.ArchiveEntry) []archiveEntryJSON {
	views := make([]archiveEntryJSON, 0, len(entries))
	for _, entry := range entries {
		views = append(views, archiveEntryJSON{
			ID:               entry.ID,
			Name:             entry.Name,
			Source:           string(entry.Source),
			Tool:             string(entry.Tool),
			Kind:             string(entry.Kind),
			OriginalLocation: entry.OriginalLocation,
			OriginRepo:       entry.OriginRepo,
			ArchivedAt:       entry.ArchivedAt.UTC().Format(time.RFC3339),
		})
	}
	return views
}

func newInstallTargetJSON(target engine.InstallTarget) installTargetJSON {
	return installTargetJSON{Kind: string(target.Kind), RepoRoot: target.RepoRoot}
}

// writeJSON emits one indented JSON document with a trailing newline, so
// output is both diffable in a golden file and pipeable to jq.
func writeJSON(w io.Writer, document any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(document)
}
