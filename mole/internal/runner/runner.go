// Package runner runs Mole unattended. Mole only accepts confirmations from
// a real terminal, so it runs in a pseudo-terminal and the runner answers
// its prompts: it confirms, skips or continues as configured, and asks for
// the admin password with a native dialog when sudo wants one.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// Options configure a run.
type Options struct {
	Env []string
	// AdminForClean answers Enter (authenticate) instead of Space (skip) to
	// mo clean's "System caches need sudo" prompt.
	AdminForClean bool
	// Password asks the user for the admin password; ok is false if they
	// cancelled. retry is true after a wrong password.
	Password func(retry bool) (password string, ok bool)
	// Delay before answering a prompt, so Mole's input drain has finished.
	Delay time.Duration
	// Log, if set, receives the terminal output with escapes removed.
	Log io.Writer
}

// Result is the outcome of a run.
type Result struct {
	ExitCode  int
	Output    string // terminal output, escapes removed
	Cancelled bool   // the password dialog was cancelled
}

// prompt is something Mole waits on, matched against the output that
// arrived since the previous answer.
type prompt struct {
	name string
	re   *regexp.Regexp
}

var prompts = []prompt{
	{"confirm", regexp.MustCompile(`Enter confirm, ESC cancel:\s*$`)},
	{"sudo-choice", regexp.MustCompile(`Enter continue, Space skip:\s*$`)},
	{"continue", regexp.MustCompile(`Press Enter to continue:\s*$`)},
	{"password", regexp.MustCompile(`(?i)password(\s+for\s+\S+)?:\s*$`)},
}

// Run runs bin with args in a pseudo-terminal until it exits.
func Run(ctx context.Context, bin string, args []string, opt Options) (Result, error) {
	if opt.Delay == 0 {
		opt.Delay = 400 * time.Millisecond
	}
	cmd := exec.Command(bin, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = append(append([]string{}, opt.Env...), "TERM=xterm-256color")
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 50, Cols: 120})
	if err != nil {
		return Result{}, fmt.Errorf("start %s: %w", bin, err)
	}
	defer tty.Close()

	var (
		mu      sync.Mutex
		all     bytes.Buffer // raw output
		pending bytes.Buffer // stripped output since the last answer
		changed = make(chan struct{}, 1)
		done    = make(chan struct{})
	)
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := tty.Read(buf)
			if n > 0 {
				mu.Lock()
				all.Write(buf[:n])
				pending.WriteString(Strip(string(buf[:n])))
				mu.Unlock()
				select {
				case changed <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return // EIO once Mole and its children have exited
			}
		}
	}()

	kill := func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
	}
	res := Result{}
	wrongPassword := false
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			kill()
			<-done
			_ = cmd.Wait()
			return res, ctx.Err()
		case <-done:
			break loop
		case <-changed:
		case <-ticker.C:
		}

		mu.Lock()
		tail := pending.String()
		mu.Unlock()
		p := match(tail)
		if p == "" {
			continue
		}
		// Let Mole finish drawing the prompt and draining stray input.
		time.Sleep(opt.Delay)
		mu.Lock()
		if pending.Len() != len(tail) && match(pending.String()) != p {
			mu.Unlock()
			continue // more output arrived; not waiting on this prompt
		}
		pending.Reset()
		mu.Unlock()

		var answer string
		switch p {
		case "confirm", "continue":
			answer = "\r"
		case "sudo-choice":
			answer = " "
			if opt.AdminForClean {
				answer = "\r"
			}
		case "password":
			if opt.Password == nil {
				res.Cancelled = true
				kill()
				continue
			}
			pw, ok := opt.Password(wrongPassword || strings.Contains(tail, "Sorry, try again"))
			if !ok {
				res.Cancelled = true
				kill()
				continue
			}
			wrongPassword = true // a further prompt means this one was wrong
			answer = pw + "\r"
		}
		if _, err := io.WriteString(tty, answer); err != nil && !errors.Is(err, os.ErrClosed) {
			kill()
		}
	}

	err = cmd.Wait()
	res.Output = Clean(all.String())
	if opt.Log != nil {
		_, _ = io.WriteString(opt.Log, res.Output)
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return res, err
		}
		res.ExitCode = exit.ExitCode()
	}
	return res, nil
}

func match(tail string) string {
	tail = strings.TrimRight(tail, " \t")
	for _, p := range prompts {
		if p.re.MatchString(tail) {
			return p.name
		}
	}
	return ""
}

var escapes = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78]`)

// Strip removes terminal escape sequences.
func Strip(s string) string {
	return escapes.ReplaceAllString(s, "")
}

// Clean turns raw terminal output into plain lines: escapes removed, and
// for lines redrawn with carriage returns (spinners) only the final text.
func Clean(s string) string {
	s = Strip(strings.ReplaceAll(s, "\r\n", "\n"))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.LastIndex(strings.TrimRight(l, "\r"), "\r"); j >= 0 {
			l = l[j+1:]
		}
		lines[i] = strings.TrimRight(l, " \r\t")
	}
	return strings.Join(lines, "\n")
}

// Summary is what Mole reports at the end of a run.
type Summary struct {
	Heading string
	Details []string
}

// ParseSummary returns the last block Mole prints between two divider lines
// of "=" (print_summary_block), if any.
func ParseSummary(out string) (Summary, bool) {
	lines := strings.Split(out, "\n")
	end := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if isDivider(lines[i]) {
			if end < 0 {
				end = i
				continue
			}
			var block []string
			for _, l := range lines[i+1 : end] {
				if l = strings.TrimSpace(l); l != "" {
					block = append(block, l)
				}
			}
			if len(block) == 0 {
				return Summary{}, false
			}
			return Summary{Heading: block[0], Details: block[1:]}, true
		}
	}
	return Summary{}, false
}

func isDivider(l string) bool {
	l = strings.TrimSpace(l)
	return len(l) >= 10 && strings.Trim(l, "=") == ""
}

// errorLine finds Mole's error lines (log_error prints "☻ message").
var errorLine = regexp.MustCompile(`^\s*(☻|✗|Error:)\s*(.+)$`)

// LastError returns the last error line Mole printed, without its marker.
func LastError(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if m := errorLine.FindStringSubmatch(lines[i]); m != nil {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

// noise is output that never explains a result: Mole's update hint and
// spinner frames.
var noise = regexp.MustCompile(`^(Update \S+ available|[⠀-⣿]\s)`)

// LastLine returns the last meaningful non-empty line.
func LastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" && !isDivider(l) && !noise.MatchString(l) {
			return l
		}
	}
	return ""
}
