package agent

import "context"

// Agent runs the reasoning loop: generate → extract tool call → execute → repeat.
type Agent struct{}

// Run starts the agent loop, blocking until done or ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	return nil
}
