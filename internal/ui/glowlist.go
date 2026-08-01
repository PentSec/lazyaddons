package ui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/list"
)

// Glow colors, taken from the lipgloss glow list example: the
// selected item is highlighted with a bright pink, the rest are
// dim grey.
var (
	glowHighlight = lipgloss.Color("#EE6FF8")
	glowDim       = lipgloss.Color("250")
)

// glowList builds a list styled like the lipgloss "glow" example:
// the selected item carries a pipe marker and the highlight color,
// the rest render dim. With airy=true items get a margin below for
// breathing room; airy=false stacks them tightly.
func glowList(selected int, airy bool) *list.List {
	base := lipgloss.NewStyle().MarginLeft(1)
	if airy {
		base = base.MarginBottom(1)
	}
	itemStyle := func(_ list.Items, i int) lipgloss.Style {
		if i == selected {
			return base.Foreground(glowHighlight)
		}
		return base.Foreground(glowDim)
	}
	enumStyle := func(_ list.Items, i int) lipgloss.Style {
		if i == selected {
			return lipgloss.NewStyle().Foreground(glowHighlight)
		}
		return lipgloss.NewStyle().Foreground(glowDim)
	}
	return list.New().
		Enumerator(func(_ list.Items, i int) string {
			if i == selected {
				return "│ "
			}
			return "  "
		}).
		ItemStyleFunc(itemStyle).
		EnumeratorStyleFunc(enumStyle)
}
