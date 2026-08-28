package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Color palette (no emojis, clean modern terminal palette)
	primaryColor = lipgloss.Color("#00D7D7") // Cyan
	accentColor  = lipgloss.Color("#A855F7") // Purple
	successColor = lipgloss.Color("#10B981") // Emerald green
	dangerColor  = lipgloss.Color("#F43F5E") // Rose/Red
	textColor    = lipgloss.Color("#F8FAFC") // Off-white
	mutedColor   = lipgloss.Color("#94A3B8") // Gray

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

	itemStyle = lipgloss.NewStyle().
			Foreground(textColor).
			PaddingLeft(2)

	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				PaddingLeft(0)

	cursorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)

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
