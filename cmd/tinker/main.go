package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nilpointr/tinker/internal/agent"
	"github.com/nilpointr/tinker/internal/llm"
	"github.com/nilpointr/tinker/internal/tools"
	"github.com/nilpointr/tinker/internal/tui"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tinker: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient("qwen2.5-coder:14b")
	extractor := llm.PromptExtractor{}
	registry := tools.NewRegistry(wd)
	ag := agent.New(client, extractor, registry)

	// pp is assigned after tea.NewProgram so goroutines started inside
	// the TUI can call (*pp).Send() to inject messages.
	var p *tea.Program
	m := tui.New(ag, &p)
	p = tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tinker: %v\n", err)
		os.Exit(1)
	}
}
