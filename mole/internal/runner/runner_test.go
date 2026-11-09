package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fake writes a bash script standing in for mo.
func fake(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mo")
	if err := os.WriteFile(p, []byte("#!/bin/bash\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func run(t *testing.T, bin string, opt Options) Result {
	t.Helper()
	opt.Delay = 50 * time.Millisecond
	res, err := Run(t.Context(), bin, nil, opt)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// The confirmation is answered with Enter, as Mole's own prompt reads it.
func TestConfirm(t *testing.T) {
	bin := fake(t, `[ -t 0 ] || { echo "not a terminal"; exit 3; }
echo "Files to be removed:"
echo -ne "\033[0;35m➤\033[0m Remove 1 app, 8KB  \033[0;32mEnter\033[0m confirm, \033[0;90mESC\033[0m cancel: "
IFS= read -r -s -n1 key
case "$key" in ""|$'\n'|$'\r') ;; *) echo "got [$key]"; exit 2 ;; esac
echo
printf '\n%s\nUninstall complete\nRemoved 1 app · 8KB: Test\n%s\n' "$(printf '=%.0s' {1..70})" "$(printf '=%.0s' {1..70})"
echo "Update 1.70.0 available, run mo update"
`)
	res := run(t, bin, Options{})
	if res.ExitCode != 0 {
		t.Fatalf("exit %d, output:\n%s", res.ExitCode, res.Output)
	}
	sum, ok := ParseSummary(res.Output)
	if !ok || sum.Heading != "Uninstall complete" || len(sum.Details) != 1 || sum.Details[0] != "Removed 1 app · 8KB: Test" {
		t.Errorf("summary = %+v, %v", sum, ok)
	}
	if strings.Contains(res.Output, "\x1b") {
		t.Error("output should have escapes removed")
	}
}

// mo clean's "System caches need sudo" prompt is skipped unless admin
// cleaning is enabled.
func TestCleanSudoChoice(t *testing.T) {
	bin := fake(t, `echo -ne "➤ System caches need sudo. Enter continue, Space skip: "
IFS= read -r -s -n1 key
[ "$key" = " " ] && echo SKIPPED || echo CONTINUED
`)
	if out := run(t, bin, Options{}).Output; !strings.Contains(out, "SKIPPED") {
		t.Errorf("default should skip:\n%s", out)
	}
	if out := run(t, bin, Options{AdminForClean: true}).Output; !strings.Contains(out, "CONTINUED") {
		t.Errorf("AdminForClean should continue:\n%s", out)
	}
}

// sudo's password prompt goes to the dialog; a wrong password asks again.
func TestPassword(t *testing.T) {
	bin := fake(t, `for i in 1 2 3; do
	printf 'Password:'
	IFS= read -r -s pw
	echo
	[ "$pw" = "right" ] && { echo "AUTH OK"; exit 0; }
	echo "Sorry, try again."
done
exit 1
`)
	var retries []bool
	answers := []string{"wrong", "right"}
	res := run(t, bin, Options{Password: func(retry bool) (string, bool) {
		retries = append(retries, retry)
		a := answers[0]
		answers = answers[1:]
		return a, true
	}})
	if res.ExitCode != 0 || !strings.Contains(res.Output, "AUTH OK") {
		t.Fatalf("exit %d, output:\n%s", res.ExitCode, res.Output)
	}
	if len(retries) != 2 || retries[0] || !retries[1] {
		t.Errorf("retry flags = %v, want [false true]", retries)
	}
	if strings.Contains(res.Output, "right") || strings.Contains(res.Output, "wrong") {
		t.Error("password must not be echoed into the output")
	}
}

// Cancelling the password dialog stops Mole.
func TestPasswordCancelled(t *testing.T) {
	bin := fake(t, `printf 'Password:'
IFS= read -r -s pw
echo "SHOULD NOT GET HERE"
`)
	res := run(t, bin, Options{Password: func(bool) (string, bool) { return "", false }})
	if !res.Cancelled || strings.Contains(res.Output, "SHOULD NOT") {
		t.Errorf("cancelled=%v output:\n%s", res.Cancelled, res.Output)
	}
}

func TestClean(t *testing.T) {
	raw := "⠋ Scanning\r⠙ Scanning\r\x1b[K✓ Done\r\nnext \x1b[1;32mline\x1b[0m\r\n"
	if got := Clean(raw); got != "✓ Done\nnext line\n" {
		t.Errorf("Clean = %q", got)
	}
}

func TestErrorLines(t *testing.T) {
	out := "Warning: No application found matching 'x'\n☻ Admin access denied\nNo matching applications found.\nUpdate 1.70.0 available, run mo update\n"
	if got := LastError(out); got != "Admin access denied" {
		t.Errorf("LastError = %q", got)
	}
	if got := LastLine(out); got != "No matching applications found." {
		t.Errorf("LastLine = %q", got)
	}
}
