package zellij

import (
	"testing"
	"time"
)

func TestParseSessions(t *testing.T) {
	out := "agent-readiness [Created 6days 21h 20m 41s ago] (EXITED - attach to resurrect)\n" +
		"iac [Created 1h 4m 21s ago] \n" +
		"seeker [Created 1h 3m 41s ago] (current)\n" +
		"\x1b[32;1mwith space\x1b[m [Created \x1b[35;1m5s\x1b[m ago] \n"
	got := ParseSessions(out)
	if len(got) != 4 {
		t.Fatalf("got %d sessions, want 4: %+v", len(got), got)
	}
	want := []Session{
		{Name: "agent-readiness", Exited: true, AgeText: "6d 21h", Age: 6*24*time.Hour + 21*time.Hour + 20*time.Minute + 41*time.Second},
		{Name: "iac", AgeText: "1h 4m", Age: time.Hour + 4*time.Minute + 21*time.Second},
		{Name: "seeker", AgeText: "1h 3m", Age: time.Hour + 3*time.Minute + 41*time.Second},
		{Name: "with space", AgeText: "5s", Age: 5 * time.Second},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("session %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSessionsEmpty(t *testing.T) {
	if got := ParseSessions("No active zellij sessions found.\n"); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

func TestPreviewPane(t *testing.T) {
	tabs := []Tab{{TabID: 0, Name: "a"}, {TabID: 1, Name: "b", Active: true}}
	panes := []Pane{
		{ID: 0, IsPlugin: true, IsFocused: true, TabID: 1},
		{ID: 1, TabID: 0, IsFocused: true},
		{ID: 2, TabID: 1},
		{ID: 3, TabID: 1, IsFocused: true},
		{ID: 4, TabID: 1, IsFocused: true, Exited: true},
	}
	p, ok := PreviewPane(tabs, panes)
	if !ok || p.ID != 3 {
		t.Fatalf("PreviewPane = %+v, %v; want pane 3", p, ok)
	}
	if _, ok := PreviewPane(nil, []Pane{{IsPlugin: true}}); ok {
		t.Fatal("plugin-only session should have no preview pane")
	}
}

func TestShellJoin(t *testing.T) {
	got := ShellJoin("/opt/homebrew/bin/zellij", "attach", "--create", "it's mine")
	want := `/opt/homebrew/bin/zellij attach --create 'it'\''s mine'`
	if got != want {
		t.Fatalf("ShellJoin = %s, want %s", got, want)
	}
}

func TestAttachCommand(t *testing.T) {
	c := &CLI{Bin: "zellij"}
	if got := c.AttachCommand("x", "compact", false); got != "zellij --new-session-with-layout compact --session x" {
		t.Errorf("new with layout: %s", got)
	}
	if got := c.AttachCommand("x", "compact", true); got != "zellij attach --create x" {
		t.Errorf("existing ignores layout: %s", got)
	}
}

func TestValidateName(t *testing.T) {
	for _, bad := range []string{"", "  ", "a/b", "a\tb"} {
		if ValidateName(bad) == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
	if err := ValidateName("my-project.2"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
}

func TestMergePath(t *testing.T) {
	got := MergePath("/usr/bin:/bin", []string{"/opt/homebrew/bin", "/usr/bin", ""})
	if got != "/usr/bin:/bin:/opt/homebrew/bin" {
		t.Fatalf("MergePath = %s", got)
	}
}
