# CLI Coding Agent — Design Doc (v1)

## Goal

Build a basic CLI coding agent from scratch, in Go, that wraps a local Ollama
model. The point of the project is to understand agent harness mechanics —
the reasoning loop, tool calling, context management — not to produce a
polished product. Prefer transparency and "build it yourself" over relying on
existing agent frameworks.

## Tech Stack

- **Language**: Go
- **TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) +
  [Lipgloss](https://github.com/charmbracelet/lipgloss) for styling, plus
  `bubbles` components (`textinput` → `textarea`, `viewport`)
- **Model runtime**: Ollama, local
  - Primary model: `qwen2.5-coder:14b`
  - Fast-iteration model (for debugging harness/parsing logic):
    `qwen2.5-coder:7b`
  - Explicit `num_ctx` override required (8192–16384) — Ollama's default
    context window is too small for a coding agent's tool-call history and
    file contents.
- **Tool calling**: Prompt-based JSON tool calls, not Ollama's native
  function-calling support. Native tool calling is more reliable but hides
  the mechanics; the explicit goal here is to see and own every part of the
  loop (prompt construction → raw model output → parsing → repair).
  Architecture should keep the "extract tool call from model output" step
  abstracted behind an interface, so a native-tool-calling strategy could be
  added later without a rewrite.

## Agent Loop (core architecture)

```
while not done:
    response = model.generate(messages)
    tool_call = extract_tool_call(response)   // strategy: prompt-based parsing
    if tool_call == done_signal:
        done = True
    elif tool_call != nil:
        result = execute_tool(ctx, tool_call)  // ctx must support cancellation
        messages.append(tool_call, result)
    else:
        // malformed output — repair/retry logic
```

Important: thread `context.Context` through the agent loop and tool
execution from the very first version, even before the cancel keybinding is
wired up in the TUI. Retrofitting cancellation later requires touching every
function signature in the call chain — cheap now, expensive later.

## Tool-Call Protocol

- Each turn may contain a prose reasoning preamble followed by a tool call
  block. The parser extracts the **last** ` ```tool ``` ` fenced block in
  the response and ignores everything before it. This lets the model explain
  its reasoning inline (important for meaningful approve/deny prompts)
  without complicating the parser.
- Format: a fenced block, optionally preceded by prose:
  ````
  I'll read main.go to understand the current structure before making changes.
  ```tool
  {"name": "read_file", "args": {"path": "main.go"}}
  ```
  ````
- An explicit **`done` signal** is required — the only way to end the
  session. Format:
  ````
  ```tool
  {"name": "done", "args": {"summary": "Added error handling to main.go."}}
  ```
  ````
  The `summary` arg is optional (empty string if omitted); the TUI renders
  it as a final status line. The system prompt must state explicitly that
  `done` is the *only* way to end the session — this closes off the
  ambiguity of the model simply stopping output.
- Malformed tool-call output triggers a **repair loop**: re-prompt the model
  with the parse error and the expected format, appending the failed attempt
  and repair prompt to message history as an `assistant`/`user` pair so the
  model sees its own mistake in context. Maximum **3 repair retries**; on
  the third failure, break the loop and surface the raw model output and
  parse error to the user (visibility into failures is a feature in a
  learning project). Repair prompt template:
  ```
  Your previous response could not be parsed as a tool call.
  Parse error: {{error}}
  Respond with only a valid tool call block — no other text:
  ```tool
  {"name": "...", "args": {...}}
  ```
  ```

## v1 Tool Set

Included in v1:
- `read_file(path)`
- `write_file(path, content)`
- `list_dir(path)`

Deferred (add later, once approve/deny + cancel are proven out):
- `run_command(cmd)` — highest value but highest risk (arbitrary shell
  execution from a local model); should only be added once the
  approve/deny and cancel UX are solid, since this is the tool that most
  needs those safety rails.
- `search_files(pattern)` (ripgrep wrapper) — useful once working in larger
  real codebases, not needed to prove out harness mechanics.

## TUI Interaction Features (build order)

1. **Approve/deny** — blocking confirmation prompt before any file write
   (and later, shell command execution).
2. **Cancel** — interrupt a running tool call / model generation. Requires
   `context.Context` plumbing from day one (see Agent Loop above); the
   keybinding/UI for it is wired up at this stage.
3. **Scrolling** — `bubbles.viewport` for the chat/log pane.
4. **Multi-line input** — swap `textinput` for `bubbles.textarea` to support
   pasting code snippets.

## Agent Persona / System Prompt

- Concise; explains **why** before **what** (e.g. states which file it's
  about to edit and the reasoning, before issuing the tool call) — this
  matters because approve/deny prompts are only meaningful if there's
  context behind them.
- One tool call per turn, strictly formatted as defined above.
- Must emit the explicit `done` signal when finished.

## Session / State Handling

- **Single-shot only for v1** — no persistence/resume of conversation
  history across CLI runs. Resumable sessions add real complexity
  (serialization, storage, "which session" addressing) for little payoff
  in a learning-focused v1.
- **Working-directory sandboxing from day one** — the agent is scoped to
  the project root (cwd at launch). All file-tool paths must be validated
  to stay within that root. This is a safety rail, not just a state
  decision, and should not be deferred.
- **Naive full-history append within a single run** — every turn is
  appended to the message list with no truncation/summarization strategy
  in v1. Simplicity over correctness; context-window pressure will surface
  naturally and motivate the next iteration (summarization, truncation,
  etc.) rather than over-engineering this up front.

## Explicit Non-Goals for v1

- Multi-model / cross-model robustness (designed for `qwen2.5-coder`
  specifically, not "works well on any Ollama model").
- Native function-calling support (may be added later as an alternate
  strategy).
- Shell command execution (`run_command`).
- Cross-session persistence.
- Context window management beyond a hard cap (no summarization yet).
