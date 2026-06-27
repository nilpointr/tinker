# tinker

A terminal coding agent built in Go. Tinker hands a local [Ollama](https://ollama.com)
model a JSON tool-calling protocol and write access to your filesystem, then studies
what happens next. The goal is to learn agent harness mechanics — reasoning loop,
tool execution, context management — by building every layer from scratch, including
the part where you realize why sandboxing matters.

> **Status:** early development — the model can now do things, which is exactly
> when sandboxing starts to matter.

## Requirements

- Go 1.26+
- [Ollama](https://ollama.com) running locally
- Model pulled: `ollama pull qwen2.5-coder:14b`

## Install

```bash
git clone https://github.com/nilpointr/tinker.git
cd tinker
make build          # produces bin/tinker
```

## Usage

```bash
./bin/tinker        # launches in the current directory
```

Tinker scopes all file operations to the directory it is launched from.

## Development

```bash
make test           # run tests
make vet            # static analysis
make build          # compile
```

To run a single test:

```bash
go test -run TestName ./internal/<package>/...
```

## Architecture

Four internal packages follow the agent loop:

| Package | Responsibility |
|---|---|
| `internal/agent` | reasoning loop, message history, repair retries |
| `internal/llm` | Ollama client, tool-call extraction strategy |
| `internal/tools` | file tools, tool registry, path sandboxing |
| `internal/tui` | Bubble Tea TUI, user input, approve/deny |

See [`design.md`](design.md) for detailed design decisions.

## License

MIT
