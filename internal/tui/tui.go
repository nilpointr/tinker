package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nilpointr/tinker/internal/agent"
)

type state int

const (
	stateInput   state = iota
	stateRunning       // agent loop is running
	stateApprove       // waiting for y/n on a tool call
	stateError
)

// inspHeight is the fixed number of rows the inspector pane occupies when shown.
// mainHeight is derived from terminal height minus fixed chrome.
const inspFixedRows = 5 // divider+header+divider+status = 4, plus the split between viewports

// Internal Bubble Tea message types.

// proseMsg carries the cleaned model response (tool block stripped) for the main viewport.
type proseMsg string

// inspectorMsg carries raw or structured content for the inspector pane.
type inspectorMsg string

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

	state         state
	input         textinput.Model
	vp            viewport.Model
	vpContent     []byte
	insp          viewport.Model
	inspContent   []byte
	showInspector bool
	pending       *approveReqMsg
	err           error
	width         int
	height        int
	ready         bool
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
		if !m.ready {
			m.vp = viewport.New(msg.Width, msg.Height-2)
			m.insp = viewport.New(msg.Width, m.inspHeight())
			m.ready = true
		}
		m = m.resized()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case proseMsg:
		if string(msg) != "" {
			m = m.appendVP(fmt.Appendf(nil, "\n%s\n", string(msg)))
		}
		return m, nil

	case inspectorMsg:
		m = m.appendInsp([]byte(msg))
		return m, nil

	case approveReqMsg:
		m.state = stateApprove
		m.pending = &msg
		m = m.appendVP(approvalBlock(msg.name, msg.args))
		return m, nil

	case toolResultMsg:
		m = m.appendVP(fmt.Appendf(nil, "\n  → %s\n", string(msg)))
		m = m.appendInsp(fmt.Appendf(nil, "\n[result]\n%s\n", string(msg)))
		m.state = stateRunning
		return m, nil

	case doneMsg:
		if string(msg) != "" {
			m = m.appendVP(fmt.Appendf(nil, "\n\nDone: %s\n\n", string(msg)))
		} else {
			m = m.appendVP([]byte("\n\nDone.\n\n"))
		}
		m.state = stateInput
		m.input.Focus()
		return m, textinput.Blink

	case errMsg:
		m.err = msg.err
		m.state = stateError
		return m, nil

	default:
		var cmds []tea.Cmd
		if m.state == stateInput {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}
		if m.ready {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
}

// appendVP appends content to the main viewport and refreshes it if ready.
func (m Model) appendVP(content []byte) Model {
	m.vpContent = append(m.vpContent, content...)
	if m.ready {
		m.vp.SetContent(string(m.vpContent))
		m.vp.GotoBottom()
	}
	return m
}

// appendInsp appends content to the inspector pane and refreshes it if shown.
func (m Model) appendInsp(content []byte) Model {
	m.inspContent = append(m.inspContent, content...)
	if m.ready && m.showInspector {
		m.insp.SetContent(string(m.inspContent))
		m.insp.GotoBottom()
	}
	return m
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ctrl+d toggles inspector in all states.
	if msg.String() == "ctrl+d" {
		return m.toggleInspector()
	}

	switch m.state {
	case stateInput:
		return m.handleInputKey(msg)
	case stateApprove:
		return m.handleApproveKey(msg)
	case stateRunning, stateError:
		return m.handleActiveKey(msg)
	}

	return m, nil
}

// toggleInspector shows or hides the inspector pane in response to ctrl+d.
func (m Model) toggleInspector() (tea.Model, tea.Cmd) {
	m.showInspector = !m.showInspector
	m = m.resized()
	if m.showInspector && m.ready {
		m.insp.SetContent(string(m.inspContent))
		m.insp.GotoBottom()
	}
	return m, nil
}

// handleInputKey handles key presses while the user is typing a task.
func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m = m.appendVP(fmt.Appendf(nil, "> %s\n", task))
		m = m.startAgent(task)
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

// handleApproveKey handles y/n/ctrl+c while a tool call awaits approval.
func (m Model) handleApproveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	default:
		if m.ready {
			var cmd tea.Cmd
			m.vp, cmd = m.vp.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// handleActiveKey handles key presses while the agent is running or errored.
func (m Model) handleActiveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if m.ready {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

// startAgent launches ag.Run for task in a background goroutine, wiring its
// callbacks to send messages back into the Bubble Tea program.
func (m Model) startAgent(task string) Model {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	pp := m.pp
	ag := m.ag
	go func() {
		p := *pp
		err := ag.Run(ctx, task, agent.RunOptions{
			OnGenerateStart: func() {
				p.Send(inspectorMsg("\n─── generating ───\n"))
			},
			OnToken: func(tok string) {
				p.Send(inspectorMsg(tok))
			},
			OnProse: func(prose string) {
				p.Send(proseMsg(prose))
			},
			OnToolCall: func(name string, args map[string]any) {
				p.Send(inspectorMsg(fmt.Sprintf("\n[call] %s\n%s", name, inspectorArgs(args))))
			},
			OnRepair: func(attempt int, err error) {
				p.Send(inspectorMsg(fmt.Sprintf("\n[repair %d/%d] %v\n", attempt, 3, err)))
			},
			OnToolResult: func(r string) { p.Send(toolResultMsg(r)) },
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
			OnDone: func(s string) { p.Send(doneMsg(s)) },
		})
		if err != nil && err != context.Canceled {
			p.Send(errMsg{err: err})
		}
	}()
	return m
}

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}

	divider := strings.Repeat("─", m.width)

	var statusLine string
	switch m.state {
	case stateInput:
		statusLine = m.input.View()
	case stateRunning:
		statusLine = "  running… (ctrl+c to cancel  ctrl+d inspector)"
	case stateApprove:
		statusLine = "  [y] approve   [n] deny   (ctrl+c to cancel)"
	case stateError:
		statusLine = fmt.Sprintf("  error: %v (ctrl+c to exit)", m.err)
	}

	if m.showInspector {
		label := " inspector  ctrl+d to hide "
		var header string
		if m.width > len(label) {
			header = label + strings.Repeat("─", m.width-len(label))
		} else {
			header = label[:m.width]
		}
		return m.vp.View() + "\n" + header + "\n" + m.insp.View() + "\n" + divider + "\n" + statusLine
	}
	return m.vp.View() + "\n" + divider + "\n" + statusLine
}

// inspHeight returns the target height for the inspector pane.
func (m Model) inspHeight() int {
	h := m.height / 3
	if h < 3 {
		h = 3
	}
	return h
}

// resized recalculates viewport dimensions after a size or layout change.
func (m Model) resized() Model {
	if !m.ready {
		return m
	}
	if m.showInspector {
		ih := m.inspHeight()
		// main + header + insp + divider + status = height
		// header=1, divider=1, status=1 → fixed=3; plus 2 \n separators
		mh := m.height - ih - 5
		if mh < 1 {
			mh = 1
		}
		m.vp.Width = m.width
		m.vp.Height = mh
		m.insp.Width = m.width
		m.insp.Height = ih
	} else {
		m.vp.Width = m.width
		m.vp.Height = m.height - 2
	}
	return m
}

// approvalBlock renders a pending tool call into the content area.
func approvalBlock(name string, args map[string]any) []byte {
	var b []byte
	b = fmt.Appendf(b, "\n  ? %s\n", name)
	if p, ok := args["path"]; ok {
		b = fmt.Appendf(b, "    path: %s\n", p)
	}
	for k, v := range args {
		if k == "path" {
			continue
		}
		val := fmt.Sprint(v)
		if strings.Contains(val, "\n") {
			b = fmt.Appendf(b, "    %s:\n", k)
			for _, line := range strings.Split(val, "\n") {
				b = fmt.Appendf(b, "      %s\n", line)
			}
		} else {
			b = fmt.Appendf(b, "    %s: %s\n", k, val)
		}
	}
	b = append(b, '\n')
	return b
}

// inspectorArgs formats tool args for the inspector pane.
func inspectorArgs(args map[string]any) string {
	var b strings.Builder
	if p, ok := args["path"]; ok {
		fmt.Fprintf(&b, "  path: %s\n", p)
	}
	for k, v := range args {
		if k == "path" {
			continue
		}
		val := fmt.Sprint(v)
		if len(val) > 80 {
			val = val[:80] + "…"
		}
		fmt.Fprintf(&b, "  %s: %s\n", k, val)
	}
	return b.String()
}
