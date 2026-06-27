package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel() Model {
	return New(nil, nil)
}

// sizedModel returns a model that has received a WindowSizeMsg,
// so the viewport is initialised and View() renders properly.
func sizedModel() Model {
	m := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// --- init / sizing ---

func TestInit_ReturnsCmd(t *testing.T) {
	if newTestModel().Init() == nil {
		t.Error("Init should return a non-nil command (textinput.Blink)")
	}
}

func TestWindowSize(t *testing.T) {
	m := update(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.width != 100 || m.height != 40 {
		t.Errorf("size: got %dx%d, want 100x40", m.width, m.height)
	}
	if !m.ready {
		t.Error("ready should be true after WindowSizeMsg")
	}
}

// --- View ---

func TestView_BeforeSizeSet(t *testing.T) {
	if newTestModel().View() == "" {
		t.Error("View() should return non-empty content before WindowSizeMsg")
	}
}

func TestView_AfterSizeSet(t *testing.T) {
	v := sizedModel().View()
	if v == "" {
		t.Error("View() returned empty string")
	}
	if !strings.Contains(v, "─") {
		t.Error("View() should contain divider")
	}
}

// --- tokenMsg ---

func TestTokenMsg_AppendsToContent(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, tokenMsg("hello "))
	m = update(m, tokenMsg("world"))
	if !strings.Contains(string(m.content), "hello world") {
		t.Errorf("content: got %q, want it to contain %q", string(m.content), "hello world")
	}
}

func TestTokenMsg_PreservesNewlines(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, tokenMsg("line1\nline2\nline3"))
	if !strings.Contains(string(m.content), "line1\nline2\nline3") {
		t.Errorf("content missing expected newlines: %q", string(m.content))
	}
}

// --- toolResultMsg ---

func TestToolResultMsg(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, toolResultMsg("file.txt"))
	if !strings.Contains(string(m.content), "file.txt") {
		t.Errorf("result not in content: %q", string(m.content))
	}
}

// --- doneMsg ---

func TestDoneMsg_ReturnToInput(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, doneMsg("all done"))
	if m.state != stateInput {
		t.Errorf("state: got %d, want stateInput (ready for next task)", m.state)
	}
	if !strings.Contains(string(m.content), "all done") {
		t.Errorf("summary not in content: %q", string(m.content))
	}
}

func TestDoneMsg_EmptySummary(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, doneMsg(""))
	if m.state != stateInput {
		t.Errorf("state: got %d, want stateInput", m.state)
	}
}

// --- errMsg ---

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

// --- approve ---

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
	m := sizedModel()
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
