package tools

import (
	"context"
	"fmt"
)

// Tool is a capability the agent can invoke.
type Tool interface {
	Name() string
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// Registry holds the available tools and dispatches execution by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns a registry pre-loaded with all v1 file tools,
// each scoped to root so they cannot access paths outside it.
func NewRegistry(root string) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	r.register(ReadFile{root: root})
	r.register(WriteFile{root: root})
	r.register(ListDir{root: root})
	return r
}

func (r *Registry) register(t Tool) {
	r.tools[t.Name()] = t
}

// Execute dispatches a tool call by name.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %q", name)
	}
	return t.Execute(ctx, args)
}

// Has reports whether a tool with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.tools[name]
	return ok
}

// stringArg extracts a required string argument from an args map.
func stringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required arg %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("arg %q must be a string, got %T", key, v)
	}
	return s, nil
}
