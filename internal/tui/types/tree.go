package types

// TreeNode represents a node in an execution tree.
type TreeNode struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`   // "trigger" | "job" | "event"
	Label    string     `json:"label"`
	Plugin   string     `json:"plugin"`
	Command  string     `json:"command"`
	Status   string     `json:"status"` // "running" | "queued" | "completed" | "failed" | "delayed" | "waiting"
	Duration string     `json:"duration,omitempty"`
	Children []TreeNode `json:"children,omitempty"`
}

// TreeTarget opens an execution tree in the detail view.
type TreeTarget struct {
	RootID string
	Tree   TreeNode
}

func (TreeTarget) detailTarget() {}
