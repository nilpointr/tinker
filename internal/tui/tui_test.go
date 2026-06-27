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
		t.Error("Init should return a non-nil command")
	}
}

func TestWindowSize_SetsReady(t *testing.T) {
	m := update(newTestModel(), tea.WindowSizeMsg{Width: 100, Height: 40})
	if !m.ready {
		t.Error("ready should be true after WindowSizeMsg")
	}
	if m.width != 100 || m.height != 40 {
		t.Errorf("size: got %dx%d, want 100x40", m.width, m.height)
	}
}

// --- View ---

func TestView_BeforeSizeSet(t *testing.T) {
	if newTestModel().View() == "" {
		t.Error("View() should return non-empty content before WindowSizeMsg")
	}
}

func TestView_ContainsDivider(t *testing.T) {
	v := sizedModel().View()
	if !strings.Contains(v, "─") {
		t.Error("View() should contain divider")
	}
}

// --- proseMsg (main viewport) ---

func TestProseMsg_AppendsToMain(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, proseMsg("hello world"))
	if !strings.Contains(string(m.vpContent), "hello world") {
		t.Errorf("prose not in main content: %q", m.vpContent)
	}
}

func TestProseMsg_EmptySkipped(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	before := len(m.vpContent)
	m = update(m, proseMsg(""))
	if len(m.vpContent) != before {
		t.Error("empty prose should not append to content")
	}
}

// --- inspectorMsg (inspector pane) ---

func TestInspectorMsg_GoesToInspector(t *testing.T) {
	m := newTestModel()
	m = update(m, inspectorMsg("raw token stream"))
	if !strings.Contains(string(m.inspContent), "raw token stream") {
		t.Errorf("inspector content missing: %q", m.inspContent)
	}
	// must NOT appear in main viewport content
	if strings.Contains(string(m.vpContent), "raw token stream") {
		t.Error("inspector content should not appear in main viewport")
	}
}

// --- toolResultMsg ---

func TestToolResultMsg_AppearsInBoth(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, toolResultMsg("file.txt"))
	if !strings.Contains(string(m.vpContent), "file.txt") {
		t.Errorf("result not in main content: %q", m.vpContent)
	}
	if !strings.Contains(string(m.inspContent), "file.txt") {
		t.Errorf("result not in inspector content: %q", m.inspContent)
	}
}

// --- doneMsg ---

func TestDoneMsg_ReturnToInput(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	m = update(m, doneMsg("all done"))
	if m.state != stateInput {
		t.Errorf("state: got %d, want stateInput", m.state)
	}
	if !strings.Contains(string(m.vpContent), "all done") {
		t.Errorf("summary not in content: %q", m.vpContent)
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
	if m.state != stateError || m.err == nil {
		t.Errorf("expected stateError with err set, got state=%d err=%v", m.state, m.err)
	}
}

// --- approve ---

func TestApproveReqMsg_AppendsBlockAndSetsState(t *testing.T) {
	m := newTestModel()
	m.state = stateRunning
	resp := make(chan bool, 1)
	m = update(m, approveReqMsg{name: "read_file", args: map[string]any{"path": "foo"}, resp: resp})
	if m.state != stateApprove {
		t.Errorf("state: got %d, want stateApprove", m.state)
	}
	if !strings.Contains(string(m.vpContent), "read_file") {
		t.Errorf("approval block not in main content: %q", m.vpContent)
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

// --- inspector toggle ---

func TestInspectorToggle(t *testing.T) {
	m := sizedModel()
	if m.showInspector {
		t.Error("inspector should be hidden by default")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if !m.showInspector {
		t.Error("inspector should be visible after ctrl+d")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.showInspector {
		t.Error("inspector should be hidden after second ctrl+d")
	}
}

func TestInspectorToggle_ViewportHeightChanges(t *testing.T) {
	m := sizedModel()
	normalHeight := m.vp.Height
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.vp.Height >= normalHeight {
		t.Errorf("main viewport should shrink when inspector shown: %d >= %d", m.vp.Height, normalHeight)
	}
	if m.insp.Height <= 0 {
		t.Errorf("inspector height should be positive: %d", m.insp.Height)
	}
}

func TestView_ShowsInspectorHeader(t *testing.T) {
	m := sizedModel()
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	m = update(m, inspectorMsg("some debug info"))
	v := m.View()
	if !strings.Contains(v, "inspector") {
		t.Errorf("inspector header not in view: %q", v)
	}
}

// --- approve view ---

func TestView_ShowsApprovePrompt(t *testing.T) {
	m := sizedModel()
	resp := make(chan bool, 1)
	m = update(m, approveReqMsg{
		name: "write_file",
		args: map[string]any{"path": "out.txt", "content": "hello"},
		resp: resp,
	})
	v := m.View()
	if !strings.Contains(v, "write_file") {
		t.Errorf("tool name not in view: %q", v)
	}
	if !strings.Contains(v, "[y] approve") {
		t.Errorf("[y] approve not in status bar: %q", v)
	}
}

// --- approvalBlock ---

func TestApprovalBlock_PathFirst(t *testing.T) {
	b := approvalBlock("write_file", map[string]any{
		"path":    "foo.txt",
		"content": "bar",
	})
	s := string(b)
	pathIdx := strings.Index(s, "path:")
	contentIdx := strings.Index(s, "content:")
	if pathIdx == -1 || contentIdx == -1 {
		t.Fatalf("expected both path and content in block: %q", s)
	}
	if pathIdx > contentIdx {
		t.Errorf("expected path before content: %q", s)
	}
}

func TestApprovalBlock_MultilineContent(t *testing.T) {
	b := approvalBlock("write_file", map[string]any{
		"path":    "out.txt",
		"content": "line1\nline2\nline3",
	})
	s := string(b)
	if !strings.Contains(s, "line1") || !strings.Contains(s, "line2") {
		t.Errorf("multiline content not rendered: %q", s)
	}
}
