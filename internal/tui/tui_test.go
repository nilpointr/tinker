package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel returns a Model with no agent or program reference,
// suitable for testing Update/View without a running Tea program.
func newTestModel() Model {
	return New(nil, nil)
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestInit_ReturnsCmd(t *testing.T) {
	m := newTestModel()
	if m.Init() == nil {
		t.Error("Init should return a non-nil command (textinput.Blink)")
	}
}

func TestWindowSize(t *testing.T) {
	m := newTestModel()
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.width != 100 || m.height != 40 {
		t.Errorf("size: got %dx%d, want 100x40", m.width, m.height)
	}
}

func TestView_BeforeSizeSet(t *testing.T) {
	m := newTestModel()
	if m.View() == "" {
		t.Error("View() should return non-empty content even before WindowSizeMsg")
	}
}

func TestView_AfterSizeSet(t *testing.T) {
	m := newTestModel()
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	v := m.View()
	if v == "" {
		t.Error("View() returned empty string")
	}
	if !strings.Contains(v, "─") {
		t.Error("View() should contain divider")
	}
}

func TestTokenMsg_AppendsToLines(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, tokenMsg("hello "))
	m = update(m, tokenMsg("world"))
	if len(m.lines) == 0 {
		t.Fatal("expected at least one line")
	}
	last := m.lines[len(m.lines)-1]
	if last != "hello world" {
		t.Errorf("last line: got %q, want %q", last, "hello world")
	}
}

func TestTokenMsg_SplitsNewlines(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, tokenMsg("line1\nline2\nline3"))
	if len(m.lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(m.lines), m.lines)
	}
}

func TestToolResultMsg(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, toolResultMsg("file.txt"))
	found := false
	for _, l := range m.lines {
		if strings.Contains(l, "file.txt") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("result not in lines: %v", m.lines)
	}
}

func TestDoneMsg_TransitionsToDone(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, doneMsg("all done"))
	if m.state != stateDone {
		t.Errorf("state: got %d, want stateDone", m.state)
	}
	found := false
	for _, l := range m.lines {
		if strings.Contains(l, "all done") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("summary not in lines: %v", m.lines)
	}
}

func TestDoneMsg_EmptySummary(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, doneMsg(""))
	if m.state != stateDone {
		t.Errorf("state: got %d, want stateDone", m.state)
	}
}

func TestErrMsg_TransitionsToError(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, errMsg{err: fmt.Errorf("boom")})
	if m.state != stateError {
		t.Errorf("state: got %d, want stateError", m.state)
	}
	if m.err == nil {
		t.Error("err should be set")
	}
}

func TestApproveReqMsg_TransitionsToApprove(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	resp := make(chan bool, 1)
	m = update(m, approveReqMsg{name: "read_file", args: map[string]any{"path": "foo"}, resp: resp})
	if m.state != stateApprove {
		t.Errorf("state: got %d, want stateApprove", m.state)
	}
	if m.pending == nil {
		t.Error("pending should be set")
	}
}

func TestApproveKey_Yes(t *testing.T) {
	resp := make(chan bool, 1)
	m := newTestModel()
	m.state = stateApprove
	m.pending = &approveReqMsg{name: "read_file", args: nil, resp: resp}

	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	select {
	case v := <-resp:
		if !v {
			t.Error("expected true from approval")
		}
	default:
		t.Error("approval channel not sent")
	}
	if m.state != stateRunning {
		t.Errorf("state: got %d, want stateRunning", m.state)
	}
}

func TestApproveKey_No(t *testing.T) {
	resp := make(chan bool, 1)
	m := newTestModel()
	m.state = stateApprove
	m.pending = &approveReqMsg{name: "write_file", args: nil, resp: resp}

	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	select {
	case v := <-resp:
		if v {
			t.Error("expected false from denial")
		}
	default:
		t.Error("approval channel not sent")
	}
	if m.state != stateRunning {
		t.Errorf("state: got %d, want stateRunning", m.state)
	}
}

func TestView_ShowsApprovePrompt(t *testing.T) {
	m := newTestModel()
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.state = stateApprove
	m.pending = &approveReqMsg{
		name: "write_file",
		args: map[string]any{"path": "out.txt"},
		resp: make(chan bool, 1),
	}
	v := m.View()
	if !strings.Contains(v, "write_file") {
		t.Errorf("approve prompt not in view: %q", v)
	}
	if !strings.Contains(v, "[y/n]") {
		t.Errorf("[y/n] not in view: %q", v)
	}
}
