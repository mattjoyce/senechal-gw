package treeview

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattjoyce/ductile/internal/tui/msgs"
	"github.com/mattjoyce/ductile/internal/tui/styles"
	"github.com/mattjoyce/ductile/internal/tui/types"
)

// flatNode is a tree node flattened for display with depth info.
type flatNode struct {
	node     types.TreeNode
	depth    int
	expanded bool
	path     []int // indices into parent's Children for expand/collapse
}

// Model renders an execution tree.
type Model struct {
	root     *types.TreeNode
	flat     []flatNode
	cursor   int
	width    int
	height   int
	expanded map[string]bool // track which nodes are expanded by ID
}

func New() Model {
	return Model{
		expanded: make(map[string]bool),
	}
}

func (m *Model) SetTree(root types.TreeNode) {
	m.root = &root
	// Default: expand all
	m.expandAll(&root)
	m.rebuild()
}

func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
			}
		case "right", "l":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if len(n.node.Children) > 0 {
					m.expanded[n.node.ID] = true
					m.rebuild()
				}
			}
		case "left", "h":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if m.expanded[n.node.ID] {
					m.expanded[n.node.ID] = false
					m.rebuild()
				}
			}
		case "enter":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.node.Type == "job" && n.node.ID != "" {
					return m, func() tea.Msg {
						return msgs.OpenDetailMsg{Target: types.JobTarget{JobID: n.node.ID}}
					}
				}
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.root == nil {
		return styles.StatusInactive.Render("  (no execution tree loaded)")
	}

	var lines []string
	for i, fn := range m.flat {
		indent := strings.Repeat("  ", fn.depth)
		glyph := statusGlyph(fn.node.Status)
		glyphStyled := styles.StatusStyle(fn.node.Status).Render(glyph)

		label := fn.node.Label
		if label == "" {
			label = fn.node.ID
		}

		var parts []string
		parts = append(parts, indent+glyphStyled)
		if fn.node.Type == "job" && fn.node.ID != "" {
			parts = append(parts, fmt.Sprintf("job:%s", truncate(fn.node.ID, 12)))
		}
		parts = append(parts, label)
		if fn.node.Plugin != "" {
			parts = append(parts, fn.node.Plugin)
		}
		parts = append(parts, styles.StatusStyle(fn.node.Status).Render(fn.node.Status))
		if fn.node.Duration != "" {
			parts = append(parts, fn.node.Duration)
		}

		// Expand/collapse indicator
		if len(fn.node.Children) > 0 {
			if m.expanded[fn.node.ID] {
				parts = append(parts, styles.StatusInactive.Render("▾"))
			} else {
				parts = append(parts, styles.StatusInactive.Render(fmt.Sprintf("▸ (%d)", countDescendants(fn.node))))
			}
		}

		line := strings.Join(parts, "  ")
		if i == m.cursor {
			line = styles.SelectedRow.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// statusGlyph returns the glyph for a status per spec §11.
func statusGlyph(status string) string {
	switch status {
	case "running":
		return "◉"
	case "queued":
		return "○"
	case "completed", "succeeded":
		return "✓"
	case "failed", "timed_out", "error":
		return "✗"
	case "delayed", "waiting":
		return "⏸"
	default:
		return "●"
	}
}

func (m *Model) expandAll(node *types.TreeNode) {
	m.expanded[node.ID] = true
	for i := range node.Children {
		m.expandAll(&node.Children[i])
	}
}

func (m *Model) rebuild() {
	m.flat = nil
	if m.root != nil {
		m.flatten(*m.root, 0)
	}
	if m.cursor >= len(m.flat) {
		m.cursor = max(0, len(m.flat)-1)
	}
}

func (m *Model) flatten(node types.TreeNode, depth int) {
	m.flat = append(m.flat, flatNode{
		node:     node,
		depth:    depth,
		expanded: m.expanded[node.ID],
	})
	if m.expanded[node.ID] {
		for _, child := range node.Children {
			m.flatten(child, depth+1)
		}
	}
}

func countDescendants(node types.TreeNode) int {
	count := len(node.Children)
	for _, child := range node.Children {
		count += countDescendants(child)
	}
	return count
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
