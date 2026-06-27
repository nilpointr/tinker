# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build          # compile to bin/tinker
make test           # go test ./...
make vet            # go vet ./...

go test ./internal/agent/...                    # single package
go test -run TestExtract ./internal/llm/...     # single test
```

## Architecture

Tinker is a CLI coding agent (TUI) that drives a local Ollama model through a prompt-based tool-calling loop. The four internal packages map cleanly onto the four concerns in the agent loop:

```
cmd/tinker/main.go   →  wires packages, starts TUI
internal/tui/        →  Bubble Tea model; owns user interaction (input, approve/deny, display)
internal/agent/      →  reasoning loop: generate → extract → execute → repeat
internal/llm/        →  Ollama HTTP client + Extractor interface + prompt-based implementation
internal/tools/      →  Tool interface, registry, file tools, path sandboxing
```

### Key design decisions

**Tool-call protocol.** The model may emit prose before a tool call. The parser always takes the *last* ` ```tool ``` ` fenced block in the response and discards everything before it. Never change this to "first block" — the prose preamble is intentional (it provides context for approve/deny prompts).

**Extractor interface (`internal/llm/extractor.go`).** The `Extractor` interface is the seam between prompt-based parsing (v1) and Ollama's native function-calling (future). Keep Ollama-specific HTTP code and extraction strategy isolated in `internal/llm/`; `internal/agent/` calls the interface only.

**Repair loop.** When extraction fails, the agent re-prompts the model with the parse error, appending the failed response + repair prompt as an `assistant`/`user` pair so the model sees its own mistake. Max 3 retries, then surface the raw output and error to the user. This lives in `internal/agent/`.

**`done` signal.** `{"name": "done", "args": {"summary": "..."}}` is the *only* exit condition — the loop does not infer completion from missing tool calls. The `summary` arg is optional.

**`context.Context` threading.** Every function in the agent loop and tool execution chain accepts a `context.Context` as its first argument. This is required from the start — it enables the cancel keybinding in the TUI and cannot be retrofitted cheaply.

**Path sandboxing.** All file tool paths are validated against the working directory at launch. This lives in `internal/tools/` and must be applied before any file operation executes.

**Message history.** Naive full-history append within a single run — no truncation or summarization. Sessions are single-shot (no cross-run persistence).

### Dependency direction

```
tui → agent → llm
             → tools
```

`tui` drives `agent`; `agent` depends on `llm` and `tools`; neither `llm` nor `tools` knows about the other or the TUI.
