package conditions

// Operator identifies one supported atomic comparison.
type Operator string

const (
	OpExists Operator = "exists"
	OpEq     Operator = "eq"
	OpNeq    Operator = "neq"
	OpIn     Operator = "in"
	OpGT     Operator = "gt"
	OpGTE    Operator = "gte"
	OpLT     Operator = "lt"
	OpLTE    Operator = "lte"
)

// Condition is one structured predicate tree.
type Condition struct {
	Path  string      `yaml:"path,omitempty" json:"path,omitempty"`
	Op    Operator    `yaml:"op,omitempty" json:"op,omitempty"`
	Value any         `yaml:"value,omitempty" json:"value,omitempty"`
	All   []Condition `yaml:"all,omitempty" json:"all,omitempty"`
	Any   []Condition `yaml:"any,omitempty" json:"any,omitempty"`
	Not   *Condition  `yaml:"not,omitempty" json:"not,omitempty"`
}

// Scope provides read-only values available to condition evaluation.
type Scope struct {
	Payload map[string]any
	Context map[string]any
	Config  map[string]any
}
