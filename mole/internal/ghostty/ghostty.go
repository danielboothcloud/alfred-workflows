// Package ghostty opens commands in Ghostty through its AppleScript
// dictionary (1.3+).
package ghostty

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Running reports whether Ghostty is running.
func Running(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/pgrep", "-xq", "ghostty").Run() == nil
}

const openScript = `on run argv
	set cmd to item 1 of argv
	set mode to item 2 of argv
	tell application "Ghostty"
		set cfg to new surface configuration
		set command of cfg to cmd
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
func Open(ctx context.Context, command, mode string) error {
	if !Running(ctx) {
		out, err := exec.CommandContext(ctx, "/usr/bin/open", "-a", "Ghostty", "--args", "--initial-command="+command).CombinedOutput()
		if err != nil {
			return fmt.Errorf("launch ghostty: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", openScript, command, mode)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ghostty applescript: %s", msg)
	}
	return nil
}
