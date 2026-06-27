package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// PromptExtractor implements Extractor by scanning for the last ```tool```
// fenced block in the model response. Prose before the block is ignored,
// which lets the model explain its reasoning before issuing a tool call.
type PromptExtractor struct{}

const (
	openFence  = "```tool"
	closeFence = "```"
)

func (PromptExtractor) Extract(response string) (*ToolCall, error) {
	last := strings.LastIndex(response, openFence)
	if last == -1 {
		return nil, errors.New("no tool block found in response")
	}

	after := response[last+len(openFence):]
	after = strings.TrimLeft(after, "\r\n")

	end := strings.Index(after, closeFence)
	if end == -1 {
		return nil, errors.New("unclosed tool block")
	}

	content := strings.TrimSpace(after[:end])

	var tc ToolCall
	if err := json.Unmarshal([]byte(content), &tc); err != nil {
		return nil, fmt.Errorf("parse tool call: %w", err)
	}

	if tc.Name == "" {
		return nil, errors.New("tool call missing name")
	}

	return &tc, nil
}
