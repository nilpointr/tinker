package agent

import (
	"context"
	"fmt"

	"github.com/nilpointr/tinker/internal/llm"
	"github.com/nilpointr/tinker/internal/tools"
)

const maxRepairAttempts = 3

const systemPrompt = `You are a coding agent. Accomplish the user's task by calling tools one at a time.

Available tools:
- read_file   {"path": "<relative-path>"}         — read a file
- write_file  {"path": "<relative-path>", "content": "<text>"} — write a file
- list_dir    {"path": "<relative-path>"}         — list directory entries

When you want to call a tool, emit a fenced block with the language tag "tool" containing a JSON object:

` + "```tool" + `
{"name": "<tool-name>", "args": {<key-value pairs>}}
` + "```" + `

You may write prose before the block to reason through the problem.
The LAST ` + "`tool`" + ` block in your response is the one that will be executed.

When the task is complete, signal done:

` + "```tool" + `
{"name": "done", "args": {"summary": "<brief description of what was accomplished>"}}
` + "```" + `

Rules:
- Only call a tool when the task genuinely requires one. For conversational
  answers or questions you can answer from knowledge, respond in prose and
  then call done directly — do not call a file tool to demonstrate capability.
- Call at most one tool per response.
- All file paths are relative to the project root; you cannot access paths outside it.
- You MUST end every response with either a tool call or done. done is the
  only way to end the session.`

const repairPrompt = `Your previous response could not be parsed as a tool call: %s

Please respond with a valid ` + "`tool`" + ` fenced block.`

// Chatter is the interface the agent uses to generate responses.
// *llm.Client satisfies this interface.
type Chatter interface {
	Chat(ctx context.Context, messages []llm.Message, onToken func(string)) (llm.Message, error)
}

// RunOptions configures callbacks for the agent loop.
// All fields are optional; nil callbacks are silently skipped.
type RunOptions struct {
	// OnToken is called for each token as the model streams its response.
	OnToken func(string)
	// OnToolCall is called before a tool is executed, with the tool name and args.
	OnToolCall func(name string, args map[string]any)
	// OnToolResult is called after a tool executes, with the result string.
	OnToolResult func(result string)
	// ShouldApprove is called before each tool execution. Return false to skip
	// execution; the agent will surface a "denied" result to the model instead.
	ShouldApprove func(name string, args map[string]any) bool
	// OnDone is called when the model signals completion, with its summary.
	OnDone func(summary string)
}

// Agent runs the generate → extract → execute reasoning loop.
type Agent struct {
	chatter   Chatter
	extractor llm.Extractor
	registry  *tools.Registry
	messages  []llm.Message
}

// New creates an Agent ready to run tasks.
func New(chatter Chatter, extractor llm.Extractor, registry *tools.Registry) *Agent {
	return &Agent{
		chatter:   chatter,
		extractor: extractor,
		registry:  registry,
		messages:  []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt}},
	}
}

// Run executes the agent loop for the given task, blocking until the model
// signals done, the context is cancelled, or an unrecoverable error occurs.
func (a *Agent) Run(ctx context.Context, task string, opts RunOptions) error {
	a.messages = append(a.messages, llm.Message{Role: llm.RoleUser, Content: task})

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		assistantMsg, err := a.chatter.Chat(ctx, a.messages, opts.OnToken)
		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}
		a.messages = append(a.messages, assistantMsg)

		call, err := a.extractWithRepair(ctx, assistantMsg.Content, opts.OnToken)
		if err != nil {
			return fmt.Errorf("extract tool call: %w", err)
		}

		if call.Name == "done" {
			summary, _ := call.Args["summary"].(string)
			if opts.OnDone != nil {
				opts.OnDone(summary)
			}
			return nil
		}

		if opts.OnToolCall != nil {
			opts.OnToolCall(call.Name, call.Args)
		}

		var result string
		if opts.ShouldApprove != nil && !opts.ShouldApprove(call.Name, call.Args) {
			result = "tool call denied by user"
		} else {
			result, err = a.registry.Execute(ctx, call.Name, call.Args)
			if err != nil {
				result = fmt.Sprintf("error: %s", err)
			}
		}

		if opts.OnToolResult != nil {
			opts.OnToolResult(result)
		}

		a.messages = append(a.messages, llm.Message{
			Role:    llm.RoleUser,
			Content: result,
		})
	}
}

// extractWithRepair attempts to extract a tool call from response, re-prompting
// the model up to maxRepairAttempts times if parsing fails.
func (a *Agent) extractWithRepair(ctx context.Context, response string, onToken func(string)) (*llm.ToolCall, error) {
	call, err := a.extractor.Extract(response)
	if err == nil {
		return call, nil
	}

	for attempt := 0; attempt < maxRepairAttempts; attempt++ {
		repair := fmt.Sprintf(repairPrompt, err)
		// Append the failed assistant turn and the repair request as a user turn
		// so the model sees exactly what went wrong.
		a.messages = append(a.messages,
			llm.Message{Role: llm.RoleAssistant, Content: response},
			llm.Message{Role: llm.RoleUser, Content: repair},
		)

		var repairMsg llm.Message
		repairMsg, err = a.chatter.Chat(ctx, a.messages, onToken)
		if err != nil {
			return nil, fmt.Errorf("repair chat: %w", err)
		}
		a.messages = append(a.messages, repairMsg)
		response = repairMsg.Content

		call, err = a.extractor.Extract(response)
		if err == nil {
			return call, nil
		}
	}

	return nil, fmt.Errorf("failed to extract tool call after %d repair attempts: %w", maxRepairAttempts, err)
}

