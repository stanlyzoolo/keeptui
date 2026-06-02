package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lepeshko/keys/internal/loader"
)

func StatusStyle(s loader.Status) lipgloss.Style {
	switch s {
	case loader.StatusActive:
		return StatusStyleActive
	case loader.StatusTrying:
		return StatusStyleTrying
	case loader.StatusForgotten:
		return StatusStyleForgotten
	case loader.StatusArchived:
		return StatusStyleArchived
	default:
		return StatusStyleTrying
	}
}
