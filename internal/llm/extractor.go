package llm

// ToolCall represents a parsed tool invocation from model output.
type ToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// Extractor parses a raw model response into a ToolCall.
// The prompt-based implementation is the v1 strategy; a native-tool-calling
// strategy can be added later by implementing this interface.
type Extractor interface {
	Extract(response string) (*ToolCall, error)
}
