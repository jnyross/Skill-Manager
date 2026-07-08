package engine

type Roots struct {
	ClaudeHome string
	CodexHome  string
	AgentsHome string
	DataDir    string
}

type Engine struct {
	roots Roots
}

func New(roots Roots) *Engine {
	return &Engine{roots: roots}
}
