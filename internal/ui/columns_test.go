package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
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

// seqNames returns n distinct names: prefix00, prefix01, ...
func seqNames(prefix string, n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("%s%02d", prefix, i)
	}
	return names
}

// assertResizeKeepsSelectionVisible scrolls a loaded table view (at least 31
// rows named by seqNames("pkg", ...), 20 tall) down to pkg30, widens the pane,
// and asserts the selected row is still on screen: clearing or shrinking a
// table's rows resets its scroll offset and would strand the cursor.
func assertResizeKeepsSelectionVisible(t *testing.T, m child) {
	t.Helper()
	for range 30 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if !strings.Contains(m.View(), "pkg30") {
		t.Fatalf("precondition: selected row should be visible before resizing:\n%s", m.View())
	}
	m, _ = m.Update(contentSizeMsg{width: 120, height: 20})
	if !strings.Contains(m.View(), "pkg30") {
		t.Errorf("selected row pkg30 scrolled out of view after resize:\n%s", m.View())
	}
}
