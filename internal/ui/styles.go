package ui

import "github.com/charmbracelet/lipgloss"

var (
	// Palette — every color used in the UI is named here; styles below build
	// only from these (no inline hex literals).
	ColorPrimary   = lipgloss.Color("#DA7756")
	ColorOrange    = lipgloss.Color("#E5A040")
	ColorOrangeDim = lipgloss.Color("#7A5A1E") // darker yellow — the API-usage gauge's empty track
	ColorGreen     = lipgloss.Color("#6AAF6A")
	ColorMeta      = lipgloss.Color("#5588AA")
	ColorMuted     = lipgloss.Color("#AAAAAA")
	ColorDim       = lipgloss.Color("#888888")
	ColorBorder    = lipgloss.Color("#555555")
	ColorText      = lipgloss.Color("#E8E8E8")
	ColorCategory  = lipgloss.Color("#E8A87C")
	ColorKey       = lipgloss.Color("#C8A97E")
	ColorDanger    = lipgloss.Color("#D06060")

	PanelBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	PanelBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)

	// SelectionBarStyle renders the ● circle indicator on selected rows
	SelectionBarStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	DescStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	SearchPromptStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	SearchMatchStyle = lipgloss.NewStyle().
				Foreground(ColorKey).
				Bold(true)

	GithubStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Rate-usage gauge (status-bar right corner): yellow brackets + used-count,
	// a filled track in the same yellow, and a darker-yellow empty track. Colors
	// are constant — the bar never recolors on rate pressure.
	RateBracketStyle    = lipgloss.NewStyle().Foreground(ColorOrange)
	RateUsageNumStyle   = lipgloss.NewStyle().Foreground(ColorOrange)
	RateGaugeFillStyle  = lipgloss.NewStyle().Background(ColorOrange)
	RateGaugeTrackStyle = lipgloss.NewStyle().Background(ColorOrangeDim)

	// WarnStyle / DangerStyle flag GitHub API rate-limit pressure in the
	// status bar and API-status overlay.
	WarnStyle = lipgloss.NewStyle().
			Foreground(ColorOrange).
			Bold(true)

	DangerStyle = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	UpdateAvailableStyle = lipgloss.NewStyle().
				Foreground(ColorOrange).
				Bold(true)

	// My Tools status colors
	StatusColorActive    = ColorGreen
	StatusColorTrying    = ColorOrange
	StatusColorForgotten = ColorMuted
	StatusColorArchived  = ColorBorder

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
			Foreground(ColorMeta)

	MetaTagStyle = lipgloss.NewStyle().
			Foreground(ColorMeta)

	MetaNoteStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	MetaDetailLabelStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Width(8)

	RepoStatusStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// SectionLabelStyle renders the bracketed section headers in the brief
	// panel, e.g. "[info]". Bold + category color to stand out from the line.
	SectionLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCategory)

	// InfoStyle is the non-italic muted style for the [info] section lines.
	InfoStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Scrollbar thumb: peach when the panel is focused, dim otherwise.
	ScrollThumbStyle = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	ScrollThumbDimStyle = lipgloss.NewStyle().
				Foreground(ColorDim)
)
