package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.Color("62")
	colDim    = lipgloss.Color("241")
	colOK     = lipgloss.Color("42")
	colErr    = lipgloss.Color("196")
	colWarn   = lipgloss.Color("214")
	colKey    = lipgloss.Color("39")
	colRef    = lipgloss.Color("213")

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(colAccent).Padding(0, 1)
	styleCrumb    = lipgloss.NewStyle().Foreground(colDim)
	styleCrumbSep = lipgloss.NewStyle().Foreground(colDim)
	styleStatus   = lipgloss.NewStyle().Foreground(colDim)
	styleOK       = lipgloss.NewStyle().Foreground(colOK)
	styleErr      = lipgloss.NewStyle().Foreground(colErr).Bold(true)
	styleWarn     = lipgloss.NewStyle().Foreground(colWarn)
	styleKey      = lipgloss.NewStyle().Foreground(colKey)
	styleRef      = lipgloss.NewStyle().Foreground(colRef).Underline(true)
	styleDim      = lipgloss.NewStyle().Foreground(colDim)
	styleBold     = lipgloss.NewStyle().Bold(true)
	styleCursor   = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	styleHelpKey  = lipgloss.NewStyle().Foreground(colKey).Bold(true)
	styleHelpBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent).Padding(1, 2)
	styleLabel    = lipgloss.NewStyle().Foreground(colDim).Width(14)
	styleLabelHot = lipgloss.NewStyle().Foreground(colKey).Bold(true).Width(14)
	styleSection  = lipgloss.NewStyle().Bold(true).Foreground(colAccent).MarginTop(1)
)
