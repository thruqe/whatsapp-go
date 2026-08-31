package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Terminal-aware grayscale palette so the TUI stays black/white while respecting light/dark themes.
	primaryColor = lipgloss.AdaptiveColor{Light: "#111111", Dark: "#F5F5F5"}
	accentColor  = lipgloss.AdaptiveColor{Light: "#333333", Dark: "#D4D4D4"}
	successColor = lipgloss.AdaptiveColor{Light: "#171717", Dark: "#E5E5E5"}
	dangerColor  = lipgloss.AdaptiveColor{Light: "#111111", Dark: "#FAFAFA"}
	textColor    = lipgloss.AdaptiveColor{Light: "#111111", Dark: "#F5F5F5"}
	mutedColor   = lipgloss.AdaptiveColor{Light: "#525252", Dark: "#A3A3A3"}

	// Typography & Container Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Padding(0, 1)

	headerBox = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1).
			MarginBottom(1)

	headerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	headerMutedStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	headerTimeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentColor)

	activeDotStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

	inactiveDotStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	activeItemStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(textColor)

	inactiveItemStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			MarginTop(1).
			PaddingLeft(2)

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(dangerColor).
			MarginTop(1).
			PaddingLeft(2)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(successColor).
			MarginTop(1).
			PaddingLeft(2)

	inputPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				MarginTop(1).
				PaddingLeft(2)
)
