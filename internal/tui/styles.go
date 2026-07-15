package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

var (
	personalHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"})
	pluginHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#5B21B6", Dark: "#C4B5FD"})
	codexHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#1D4ED8", Dark: "#93C5FD"})
	projectHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#047857", Dark: "#86EFAC"})

	skillRowStyle         = lipgloss.NewStyle()
	selectedSkillRowStyle = lipgloss.NewStyle().Bold(true)
	skillMetaStyle        = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"})

	detailPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#475569"}).
			Padding(0, 1)
	detailTitleStyle = lipgloss.NewStyle().
				Bold(true)
	detailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94A3B8"})

	confirmBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94A3B8"}).
			Padding(1, 2)
	confirmBackdropStyle = lipgloss.NewStyle().
				Faint(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#64748B"})

	activationAutoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#334155", Dark: "#CBD5E1"})
	activationManualOnlyStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#92400E", Dark: "#FBBF24"})
	activationSuppressedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"})
	activationDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"})
)

func sourceHeaderStyle(source engine.Source) lipgloss.Style {
	switch source {
	case engine.SourcePersonal:
		return personalHeaderStyle
	case engine.SourcePlugin:
		return pluginHeaderStyle
	case engine.SourceCodex:
		return codexHeaderStyle
	case engine.SourceProject:
		return projectHeaderStyle
	default:
		return skillMetaStyle
	}
}

func activationStyle(activation engine.ActivationState) lipgloss.Style {
	switch activation {
	case engine.ActivationManualOnly:
		return activationManualOnlyStyle
	case engine.ActivationSuppressed:
		return activationSuppressedStyle
	case engine.ActivationDisabled:
		return activationDisabledStyle
	default:
		return activationAutoStyle
	}
}
