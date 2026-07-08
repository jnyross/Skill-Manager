package engine

type Roots struct {
	ClaudeHome         string
	CodexHome          string
	AgentsHome         string
	DataDir            string
	ProjectRoots       []string
	ClaudeProjectRoots []string
}

type Engine struct {
	roots Roots
}

func New(roots Roots) *Engine {
	return &Engine{roots: roots}
}
