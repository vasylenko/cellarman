package ui

import (
	"testing"

	"charm.land/bubbles/v2/table"
)

func TestFitCol(t *testing.T) {
	rows := []table.Row{{"go", "x"}, {"gobject-introspection", "x"}, {"日本語", "x"}}
	tests := []struct {
		name  string
		title string
		rows  []table.Row
		maxW  int
		want  int
	}{
		{"widest cell", "Formula", rows[:2], 40, 21},
		{"title wider than cells", "Formula", rows[:1], 40, 7},
		{"capped", "Formula", rows[:2], 10, 10},
		{"no rows keeps the title", "Formula", nil, 40, 7},
		{"display cells, not bytes", "F", rows[2:], 40, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fitCol(tt.title, tt.rows, 0, tt.maxW); got != tt.want {
				t.Errorf("fitCol = %d, want %d", got, tt.want)
			}
		})
	}
}

// The fill column must make the row exactly as wide as the pane once every
// column's padding is counted — one cell over and the viewport clips the edge.
func TestFillColBudgetsPadding(t *testing.T) {
	tests := []struct {
		name  string
		total int
		fixed []int
	}{
		{"one fixed column", 100, []int{20}},
		{"package layout", 80, []int{22, 10, flagColWidth}},
		{"no fixed columns", 60, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used := fillCol(tt.total, tt.fixed...) + cellPad*(len(tt.fixed)+1)
			for _, f := range tt.fixed {
				used += f
			}
			if used != tt.total {
				t.Errorf("columns + padding = %d, want %d", used, tt.total)
			}
		})
	}
}

func TestFillColFloor(t *testing.T) {
	if got := fillCol(40, 30, 10); got != minFillWidth {
		t.Errorf("fillCol = %d, want the %d floor when fixed columns overrun", got, minFillWidth)
	}
}
