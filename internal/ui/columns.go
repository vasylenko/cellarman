package ui

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// Shared table layout: columns are sized as fractions of the available width,
// clamped to a legible minimum, with a fixed-width status column.
const (
	minTableWidth = 40
	flagColWidth  = 3
	// cellPad is the horizontal padding bubbles' default table styles add around
	// every column (Padding(0, 1)). Width budgets must include it, or the
	// table's viewport clips the rightmost column.
	cellPad = 2
	// minFillWidth keeps a flexible column legible when fixed columns crowd it.
	minFillWidth = 8
)

// tableWidth clamps the available width to minTableWidth before columns are
// sized as fractions of it, so a narrow terminal doesn't collapse them.
func tableWidth(w int) int {
	if w < minTableWidth {
		return minTableWidth
	}
	return w
}

// frac returns a fraction of total as an int column width (min 4).
func frac(total int, f float64) int {
	w := int(float64(total) * f)
	if w < 4 {
		return 4
	}
	return w
}

// fitCol sizes column i to its widest cell (or title), capped at maxW so one
// long value can't starve the flexible column; longer cells truncate with "…".
func fitCol(title string, rows []table.Row, i, maxW int) int {
	w := lipgloss.Width(title)
	for _, r := range rows {
		w = max(w, lipgloss.Width(r[i]))
	}
	return min(w, maxW)
}

// fillCol is the width left for the one flexible column once the fixed
// columns, and every column's padding (its own included), are paid for.
func fillCol(total int, fixed ...int) int {
	w := total - cellPad*(len(fixed)+1)
	for _, f := range fixed {
		w -= f
	}
	return max(w, minFillWidth)
}
