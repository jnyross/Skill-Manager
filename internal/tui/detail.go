package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

type detailPane struct {
	vp      viewport.Model
	content string
	// measured caches each Skill's on-disk footprint by Location. Those two
	// numbers are the only ones that need a directory walk (see
	// engine.MeasureSkillFiles), so they are measured for the Skill the user is
	// actually looking at, once, rather than for every Skill on every refresh.
	// refreshInventory drops the cache, since that is exactly when a Skill's
	// files may have changed underneath it.
	measured map[string]diskFootprint
}

// diskFootprint is one cached measurement.
type diskFootprint struct {
	files int
	bytes int64
}

func newDetailPane() detailPane {
	return detailPane{vp: viewport.New(0, 0), measured: make(map[string]diskFootprint)}
}

// forgetMeasurements drops the cached on-disk footprints, so the next selection
// re-measures. Called whenever the inventory is re-read.
func (p *detailPane) forgetMeasurements() {
	p.measured = make(map[string]diskFootprint)
}

// withDiskFootprint fills in the Skill's FileCount and TotalBytes, from the
// cache when it can.
func (p *detailPane) withDiskFootprint(skill engine.Skill) engine.Skill {
	if skill.Location == "" {
		return skill
	}
	if p.measured == nil {
		p.measured = make(map[string]diskFootprint)
	}
	if cached, ok := p.measured[skill.Location]; ok {
		skill.FileCount = cached.files
		skill.TotalBytes = cached.bytes
		return skill
	}
	// Notices from the measurement are dropped here: the only one it can raise
	// is the file-budget bound, which the pane already communicates by showing
	// a large count.
	engine.MeasureSkillFiles(&skill)
	p.measured[skill.Location] = diskFootprint{files: skill.FileCount, bytes: skill.TotalBytes}
	return skill
}

func (p *detailPane) setSize(width, height int) {
	if width < 1 {
		width = 40
	}
	if height < 1 {
		height = 1
	}

	// Viewport fills the inside of the bordered pane.
	contentWidth := width - detailPaneStyle.GetHorizontalFrameSize()
	if contentWidth < 1 {
		contentWidth = 1
	}
	contentHeight := height - detailPaneStyle.GetVerticalFrameSize()
	if contentHeight < 1 {
		contentHeight = 1
	}
	p.vp.Width = contentWidth
	p.vp.Height = contentHeight
	// Re-apply content after size change so the viewport reclamps YOffset
	// without jumping to top when the selection is unchanged.
	if p.content != "" {
		y := p.vp.YOffset
		p.applyContent()
		p.vp.SetYOffset(y)
	}
}

// setSkill replaces detail content when the selected skill changes. Unchanged
// content is a no-op so scroll position survives unrelated Update cycles.
func (p *detailPane) setSkill(skill engine.Skill, ok bool) {
	if ok {
		skill = p.withDiskFootprint(skill)
	}
	content := detailContent(skill, ok, p.vp.Width)
	if content == p.content {
		return
	}
	p.content = content
	p.applyContent()
	p.vp.GotoTop()
}

// applyContent sets the viewport content.
func (p *detailPane) applyContent() {
	p.vp.SetContent(p.content)
}

func (p *detailPane) scrollHalf(delta int) {
	if delta > 0 {
		p.vp.HalfViewDown()
		return
	}
	if delta < 0 {
		p.vp.HalfViewUp()
	}
}

func (p detailPane) render() string {
	width := p.vp.Width
	if width < 1 {
		width = 1
	}
	height := p.vp.Height
	if height < 1 {
		height = 1
	}

	view := p.vp.View()
	if p.vp.TotalLineCount() > p.vp.Height {
		// Overlay a "more" hint on the last visible line so the user knows
		// the pane is scrollable.
		lines := strings.Split(view, "\n")
		if len(lines) > 0 {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, skillMetaStyle.Render("↓ more"))
		view = strings.Join(lines, "\n")
	}

	// lipgloss Width() counts padding but not the border, while the viewport
	// wrapped to the full content width. Passing the viewport's width straight
	// through would leave the inner area two columns narrower than the text it
	// holds, re-wrapping the longest lines and growing the pane past the height
	// it was given — which pushes the title and the cost header off the top of
	// the screen. Add the padding back so the content area matches the wrap.
	return detailPaneStyle.
		Width(width + detailPaneStyle.GetHorizontalPadding()).
		Height(height).
		Render(view)
}

func detailContent(skill engine.Skill, ok bool, width int) string {
	if !ok {
		return skillMetaStyle.Render("No skill selected.")
	}

	if width < 1 {
		width = 1
	}

	var b strings.Builder
	b.WriteString(detailTitleStyle.Render(skill.Name))
	b.WriteString("\n\n")
	writeDetailField(&b, "Source", string(skill.Source))
	if skill.Source == engine.SourceProject {
		writeDetailField(&b, "Tool", string(skill.Tool))
	}
	writeDetailField(&b, "Kind", string(skill.Kind))
	writeDetailField(&b, "Activation", activationStyle(skill.Activation).Render(string(skill.Activation)))
	if skill.Tool == engine.ToolCodex && skill.DeclaredManualOnlyForClaude && skill.Activation == engine.ActivationAuto {
		writeDetailField(&b, "Note", "SKILL.md declares manual-only for Claude Code; no effect in Codex")
	}
	writeDetailField(&b, "Description", skill.Description)
	writeDetailField(&b, "Cost (estimated)", costSection(skill))
	writeDetailWrappedField(&b, "Location", skill.Location, width)
	if skill.Source == engine.SourcePlugin && skill.Plugin != nil {
		writeDetailField(&b, "Plugin", fmt.Sprintf("one of %d in %s", skill.Plugin.SkillCount, skill.Plugin.Plugin))
	}
	return strings.TrimRight(b.String(), "\n")
}

// costSection is the detail pane's answer to "what does this Skill cost me?".
//
// It separates the two costs that behave completely differently. The per-session
// line is the standing tax: while a Skill Auto-activates, its description is
// injected into every session with its Tool whether or not the Skill is ever
// used. The invoked line is what the Skill costs only when it actually runs.
// Confusing the two is the misunderstanding this whole section exists to
// prevent, so a Skill that is not Auto shows a per-session cost of zero and
// says what turning Auto-activation back on would cost.
//
// Every number is prefixed "~" by engine.FormatTokenEstimate; the field label
// carries "(estimated)" so no reading of this pane implies exactness.
func costSection(skill engine.Skill) string {
	var b strings.Builder

	if skill.Activation == engine.ActivationAuto {
		fmt.Fprintf(&b, "Per session   %s tokens — its description is injected into every %s session\n",
			engine.FormatTokenEstimate(skill.DescriptionTokens), skill.Tool)
	} else {
		fmt.Fprintf(&b, "Per session   ~0 tokens — %s, so its description is not injected (%s tokens if set back to Auto)\n",
			skill.Activation, engine.FormatTokenEstimate(skill.DescriptionTokens))
	}

	body := "unknown"
	if skill.BodyBytes > 0 {
		body = fmt.Sprintf("%s tokens (%s %s)", engine.FormatTokenEstimate(skill.BodyTokens),
			engine.FormatByteSize(skill.BodyBytes), bodyFileLabel(skill))
	}
	fmt.Fprintf(&b, "When invoked  %s\n", body)

	if skill.FileCount > 0 {
		fmt.Fprintf(&b, "On disk       %s, %s", pluralFiles(skill.FileCount), engine.FormatByteSize(skill.TotalBytes))
	}
	return strings.TrimRight(b.String(), "\n")
}

func bodyFileLabel(skill engine.Skill) string {
	if skill.Kind == engine.KindPrompt {
		return "prompt file"
	}
	return "SKILL.md"
}

func pluralFiles(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

func writeDetailField(b *strings.Builder, label, value string) {
	if value == "" {
		value = "-"
	}
	b.WriteString(detailLabelStyle.Render(label))
	b.WriteString("\n")
	b.WriteString(value)
	b.WriteString("\n\n")
}

func writeDetailWrappedField(b *strings.Builder, label, value string, width int) {
	if value == "" {
		value = "-"
	}
	b.WriteString(detailLabelStyle.Render(label))
	b.WriteString("\n")
	wrapped := lipgloss.NewStyle().Width(width).Render(value)
	b.WriteString(wrapped)
	b.WriteString("\n\n")
}
