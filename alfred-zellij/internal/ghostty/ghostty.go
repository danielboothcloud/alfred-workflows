// Package ghostty drives Ghostty through its AppleScript dictionary (1.3+).
package ghostty

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Terminal is one Ghostty terminal surface.
type Terminal struct {
	ID      string
	TTY     string // e.g. /dev/ttys003
	Title   string
	Focused bool // focused terminal of the selected tab in the front window
}

func osascript(ctx context.Context, script string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", append([]string{"-e", script}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("ghostty applescript: %s", msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Running reports whether Ghostty is running.
func Running(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/pgrep", "-xq", "ghostty").Run() == nil
}

// "tab" is a Ghostty class, so the tab character is spelled `character id 9`.
const listScript = `on run argv
	set sep to character id 9
	tell application "Ghostty"
		set fid to ""
		try
			set fid to id of focused terminal of selected tab of front window
		end try
		set out to "FOCUSED" & sep & fid & linefeed
		repeat with t in terminals
			set ttl to ""
			try
				set ttl to (name of t) as text
			end try
			set out to out & (id of t) & sep & (tty of t) & sep & ttl & linefeed
		end repeat
		return out
	end tell
end run`

// Terminals lists all Ghostty terminals. Callers should check Running first;
// otherwise this launches Ghostty.
func Terminals(ctx context.Context) ([]Terminal, error) {
	out, err := osascript(ctx, listScript)
	if err != nil {
		return nil, err
	}
	return parseTerminals(out), nil
}

func parseTerminals(out string) []Terminal {
	var focused string
	var terms []Terminal
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) < 2 {
			continue
		}
		if f[0] == "FOCUSED" {
			focused = f[1]
			continue
		}
		t := Terminal{ID: f[0], TTY: f[1]}
		if len(f) == 3 {
			t.Title = f[2]
		}
		terms = append(terms, t)
	}
	for i := range terms {
		terms[i].Focused = focused != "" && terms[i].ID == focused
	}
	return terms
}

const focusScript = `on run argv
	tell application "Ghostty"
		focus (first terminal whose id is (item 1 of argv))
		activate
	end tell
end run`

// Focus brings the terminal's window to the front and focuses it.
func Focus(ctx context.Context, id string) error {
	_, err := osascript(ctx, focusScript, id)
	return err
}

const openScript = `on run argv
	set cmd to item 1 of argv
	set cwd to item 2 of argv
	set mode to item 3 of argv
	tell application "Ghostty"
		set cfg to new surface configuration
		set command of cfg to cmd
		if cwd is not "" then set initial working directory of cfg to cwd
		if mode is "tab" and (count of windows) > 0 then
			new tab in front window with configuration cfg
		else
			new window with configuration cfg
		end if
		activate
	end tell
end run`

// Open runs command in a new Ghostty window, or a new tab of the front window
// when mode is "tab". If Ghostty is not running it is launched with the
// command as its initial command, so no extra empty window appears.
func Open(ctx context.Context, command, cwd, mode string) error {
	if !Running(ctx) {
		args := []string{"-a", "Ghostty", "--args", "--initial-command=" + command}
		if cwd != "" {
			args = append(args, "--working-directory="+cwd)
		}
		if out, err := exec.CommandContext(ctx, "/usr/bin/open", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("launch ghostty: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	_, err := osascript(ctx, openScript, command, cwd, mode)
	return err
}
