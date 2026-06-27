package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nilpointr/tinker/internal/agent"
)

type state int

const (
	stateInput   state = iota
	stateRunning       // agent loop is running
	stateApprove       // waiting for y/n on a tool call
	stateDone
	stateError
)

// Internal Bubble Tea message types sent from the agent goroutine.

type tokenMsg string

type approveReqMsg struct {
	name string
	args map[string]any
	resp chan bool
}

type toolResultMsg string
type doneMsg string
type errMsg struct{ err error }

// Model is the Bubble Tea application model.
// pp must point to the *tea.Program that wraps this model — set it
// before calling p.Run() so background goroutines can send messages.
type Model struct {
	ag     *agent.Agent
	pp     **tea.Program
	cancel context.CancelFunc

	state   state
	input   textinput.Model
	lines   []string
	pending *approveReqMsg
	err     error
	width   int
	height  int
}

// New returns a Model ready to pass to tea.NewProgram.
func New(ag *agent.Agent, pp **tea.Program) Model {
	ti := textinput.New()
	ti.Placeholder = "What would you like me to do?"
	ti.Focus()
	return Model{
		ag:    ag,
		pp:    pp,
		state: stateInput,
		input: ti,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tokenMsg:
		parts := strings.Split(string(msg), "\n")
		for i, part := range parts {
			if i == 0 {
				if len(m.lines) == 0 {
					m.lines = append(m.lines, part)
				} else {
					m.lines[len(m.lines)-1] += part
				}
			} else {
				m.lines = append(m.lines, part)
			}
		}
		return m, nil

	case approveReqMsg:
		m.state = stateApprove
		m.pending = &msg
		return m, nil

	case toolResultMsg:
		m.lines = append(m.lines, "", "  → "+string(msg), "")
		m.state = stateRunning
		return m, nil

	case doneMsg:
		m.lines = append(m.lines, "")
		if string(msg) != "" {
			m.lines = append(m.lines, "Done: "+string(msg))
		} else {
			m.lines = append(m.lines, "Done.")
		}
		m.state = stateDone
		return m, nil

	case errMsg:
		m.err = msg.err
		m.state = stateError
		return m, nil

	default:
		// Forward unhandled messages to the text input (needed for cursor blink).
		if m.state == stateInput {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateInput:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			task := strings.TrimSpace(m.input.Value())
			if task == "" {
				return m, nil
			}
			m.state = stateRunning
			m.input.SetValue("")
			m.lines = append(m.lines, "> "+task, "")
			ctx, cancel := context.WithCancel(context.Background())
			m.cancel = cancel
			pp := m.pp
			ag := m.ag
			go func() {
				p := *pp
				err := ag.Run(ctx, task, agent.RunOptions{
					OnToken: func(tok string) { p.Send(tokenMsg(tok)) },
					ShouldApprove: func(name string, args map[string]any) bool {
						resp := make(chan bool, 1)
						p.Send(approveReqMsg{name: name, args: args, resp: resp})
						select {
						case v := <-resp:
							return v
						case <-ctx.Done():
							return false
						}
					},
					OnToolResult: func(r string) { p.Send(toolResultMsg(r)) },
					OnDone:       func(s string) { p.Send(doneMsg(s)) },
				})
				if err != nil && err != context.Canceled {
					p.Send(errMsg{err: err})
				}
			}()
			return m, nil
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case stateApprove:
		switch msg.String() {
		case "y", "Y":
			if m.pending != nil {
				m.pending.resp <- true
				m.pending = nil
			}
			m.state = stateRunning
			return m, nil
		case "n", "N":
			if m.pending != nil {
				m.pending.resp <- false
				m.pending = nil
			}
			m.state = stateRunning
			return m, nil
		case "ctrl+c":
			if m.pending != nil {
				m.pending.resp <- false
				m.pending = nil
			}
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}

	case stateRunning, stateDone, stateError:
		if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	outputHeight := m.height - 3
	if outputHeight < 1 {
		outputHeight = 1
	}

	// Take the last outputHeight committed lines, pad with empty lines at top.
	start := len(m.lines) - outputHeight
	if start < 0 {
		start = 0
	}
	visible := m.lines[start:]

	display := make([]string, outputHeight)
	offset := outputHeight - len(visible)
	for i, line := range visible {
		display[offset+i] = line
	}
	output := strings.Join(display, "\n")

	divider := strings.Repeat("─", m.width)

	var statusLine string
	switch m.state {
	case stateInput:
		statusLine = m.input.View()
	case stateRunning:
		statusLine = "  running… (ctrl+c to cancel)"
	case stateApprove:
		if m.pending != nil {
			statusLine = fmt.Sprintf("  Run %s(%s)? [y/n]",
				m.pending.name, formatArgs(m.pending.args))
		}
	case stateDone:
		statusLine = "  press ctrl+c to exit"
	case stateError:
		statusLine = fmt.Sprintf("  error: %v (ctrl+c to exit)", m.err)
	}

	return output + "\n" + divider + "\n" + statusLine
}

func formatArgs(args map[string]any) string {
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%q", k, fmt.Sprint(v)))
	}
	return strings.Join(parts, ", ")
}
