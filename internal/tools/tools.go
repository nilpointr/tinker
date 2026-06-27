package tools

import "context"

// Tool is a capability the agent can invoke.
type Tool interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) (string, error)
}
