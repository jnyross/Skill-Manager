package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const maxConfirmContentWidth = 72

// renderConfirmOverlay relies on renderConfirmBox never rendering wider than
// the width it's given — the splice below assumes boxWidth <= width and
// doesn't re-clamp.
func renderConfirmOverlay(background, description string, width, height int) string {
	width, height = confirmCanvasSize(background, width, height)
	lines := dimBackgroundLines(background, width, height)
	box := renderConfirmBox(description, width)
	boxLines := strings.Split(box, "\n")
	boxWidth := maxLineWidth(boxLines)
	boxHeight := len(boxLines)

	if boxHeight > height {
		// Too short a terminal to center the box within the canvas without
		// clipping its bottom border off — degrade to the bare box, same as
		// renderConfirmBox's width < 3 fallback does for narrow terminals.
		return box
	}

	left := (width - boxWidth) / 2
	if left < 0 {
		left = 0
	}
	top := (height - boxHeight) / 2
	if top < 0 {
		top = 0
	}

	for i, boxLine := range boxLines {
		lineIndex := top + i
		if lineIndex < 0 || lineIndex >= len(lines) {
			continue
		}

		rightStart := left + lipgloss.Width(boxLine)
		leftSide := ansi.Cut(lines[lineIndex], 0, left)
		rightSide := ""
		if rightStart < width {
			rightSide = ansi.Cut(lines[lineIndex], rightStart, width)
		}
		lines[lineIndex] = leftSide + boxLine + rightSide
	}

	return strings.Join(lines, "\n")
}

func renderConfirmBox(description string, width int) string {
	if width < 3 {
		return ansi.Truncate(description, width, "")
	}

	style := confirmBoxStyle
	if width < 12 {
		style = style.Padding(0, 0)
	}

	contentWidth := width - style.GetHorizontalFrameSize()
	if contentWidth > maxConfirmContentWidth {
		contentWidth = maxConfirmContentWidth
	}
	if contentWidth < 1 {
		contentWidth = 1
	}

	return style.Width(contentWidth).Render(description)
}

func confirmCanvasSize(background string, width, height int) (int, int) {
	if width < 1 {
		width = maxLineWidth(strings.Split(background, "\n"))
		if width < 1 {
			width = 100
		}
	}
	if height < 1 {
		height = len(strings.Split(background, "\n"))
		if height < 1 {
			height = 1
		}
	}
	return width, height
}

func dimBackgroundLines(background string, width, height int) []string {
	sourceLines := strings.Split(background, "\n")
	lines := make([]string, height)
	for i := range lines {
		line := ""
		if i < len(sourceLines) {
			line = ansi.Truncate(ansi.Strip(sourceLines[i]), width, "")
		}
		lines[i] = confirmBackdropStyle.Width(width).Render(line)
	}
	return lines
}

func maxLineWidth(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		if width := lipgloss.Width(line); width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}
