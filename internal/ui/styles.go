package ui

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary  = lipgloss.Color("#DA7756")
	ColorOrange   = lipgloss.Color("#E5A040")
	ColorMuted    = lipgloss.Color("#AAAAAA")
	ColorBorder   = lipgloss.Color("#555555")
	ColorText     = lipgloss.Color("#E8E8E8")
	ColorCategory = lipgloss.Color("#E8A87C")
	ColorKey      = lipgloss.Color("#C8A97E")

	PanelBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	PanelBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)

	// SelectionBarStyle renders the ● circle indicator on selected rows
	SelectionBarStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			PaddingLeft(1)

	DescStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(1).
			BorderTop(true).
			BorderBottom(true).
			BorderLeft(true).
			BorderRight(true).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	SearchPromptStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	SearchMatchStyle = lipgloss.NewStyle().
				Foreground(ColorKey).
				Bold(true)

	GithubStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	VersionInstalledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))

	VersionOkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6AAF6A"))

	UpdateAvailableStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E5A040")).
				Bold(true)

	// My Tools status colors
	StatusColorActive    = lipgloss.Color("#6AAF6A")
	StatusColorTrying    = lipgloss.Color("#E5A040")
	StatusColorForgotten = lipgloss.Color("#AAAAAA")
	StatusColorArchived  = lipgloss.Color("#555555")

	StatusStyleActive = lipgloss.NewStyle().
				Foreground(StatusColorActive).
				Bold(true)

	StatusStyleTrying = lipgloss.NewStyle().
				Foreground(StatusColorTrying).
				Bold(true)

	StatusStyleForgotten = lipgloss.NewStyle().
				Foreground(StatusColorForgotten)

	StatusStyleArchived = lipgloss.NewStyle().
				Foreground(StatusColorArchived)

	HelpFlagStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary)

	HelpSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCategory)

	HelpMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5588AA"))

	MetaTagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5588AA"))

	MetaNoteStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	MetaDetailLabelStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Width(8)

	MetaDetailValueStyle = lipgloss.NewStyle().
				Foreground(ColorText)

	RepoStatusStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Italic(true)
)
