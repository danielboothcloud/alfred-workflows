package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"alfred-mole/internal/alfred"
	"alfred-mole/internal/mole"
	"alfred-mole/internal/runner"
)

const (
	defaultBundleID = "com.danielbooth.alfred-mole"
	notifyTrigger   = "notify"
	logName         = "last-run.log"
	runLimit        = 30 * time.Minute
)

// background reports whether action runs unattended instead of in Ghostty.
func background(action string) bool {
	return action == "uninstall" || action == "clean"
}

func logPath() string { return filepath.Join(alfred.CacheDir(), logName) }

// spawnWork runs the action in a detached process, so Alfred is free
// straight away; the result arrives as a notification.
func spawnWork(action, arg string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "work", action, arg) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// work runs Mole unattended and reports the outcome as a notification.
func work(action, arg string) error {
	args, err := moleArgs(action, arg)
	if err != nil {
		return err
	}
	bin, err := mole.Find(os.Getenv("mole_path"))
	if err != nil {
		notify("Mole", err.Error())
		return err
	}

	// One Mole run at a time; later ones wait their turn.
	unlock, err := lockRun()
	if err != nil {
		notify("Mole", err.Error())
		return err
	}
	defer unlock()

	start := startMessage(action, arg)
	notify(start, "Running in the background…")

	logf, _ := os.Create(logPath())
	if logf != nil {
		fmt.Fprintf(logf, "$ mo %s\n# %s\n\n", shellJoin(args...), time.Now().Format(time.DateTime))
		defer logf.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), runLimit)
	defer cancel()
	opts := runner.Options{
		Env:           mole.Env(),
		AdminForClean: env("clean_admin", "0") == "1",
		Password:      func(retry bool) (string, bool) { return askPassword(action, arg, retry) },
	}
	if logf != nil {
		opts.Log = logf
	}
	res, err := runner.Run(ctx, bin, args, opts)
	title, body := outcome(action, arg, res, err)
	notify(title, body)

	if action == "uninstall" && err == nil && res.ExitCode == 0 {
		spawnRefresh() // keep the cached app list current
	}
	return nil
}

func startMessage(action, arg string) string {
	if action == "uninstall" {
		return "Uninstalling " + arg + "…"
	}
	return "Cleaning your Mac…"
}

// outcome turns a run into a notification title and body.
func outcome(action, arg string, res runner.Result, err error) (string, string) {
	what := "Clean"
	if action == "uninstall" {
		what = "Uninstall " + arg
	}
	switch {
	case err != nil && err == context.DeadlineExceeded:
		return what + " timed out", fmt.Sprintf("Stopped after %s. See the log: mole › Last run log", runLimit)
	case err != nil:
		return what + " failed", err.Error()
	case res.Cancelled:
		return what + " cancelled", "No admin password was given, so nothing was changed"
	}

	sum, hasSum := runner.ParseSummary(res.Output)
	if res.ExitCode == 0 {
		if action == "uninstall" {
			title := "Uninstalled " + arg
			if hasSum {
				return title, strings.Join(sum.Details, "\n")
			}
			return title, runner.LastLine(res.Output)
		}
		if hasSum {
			return sum.Heading, strings.Join(sum.Details, "\n")
		}
		return "Clean complete", runner.LastLine(res.Output)
	}

	msg := runner.LastError(res.Output)
	if msg == "" && hasSum {
		msg = strings.Join(append([]string{sum.Heading}, sum.Details...), "\n")
	}
	if msg == "" {
		msg = runner.LastLine(res.Output)
	}
	if msg == "" {
		msg = fmt.Sprintf("mo exited with status %d", res.ExitCode)
	}
	return what + " failed", msg
}

func lockRun() (func(), error) {
	f, err := os.OpenFile(filepath.Join(alfred.CacheDir(), "run.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

const passwordScript = `on run argv
	activate
	set r to display dialog (item 1 of argv) default answer "" with title "Mole" with icon caution with hidden answer buttons {"Cancel", "OK"} default button "OK" cancel button "Cancel"
	return text returned of r
end run`

// askPassword shows a native dialog for the admin password sudo wants.
func askPassword(action, arg string, retry bool) (string, bool) {
	msg := "Mole needs your password to clean system caches."
	if action == "uninstall" {
		msg = "Mole needs your password to uninstall " + arg + "."
	}
	if retry {
		msg = "Sorry, try again.\n\n" + msg
	}
	out, err := exec.Command("/usr/bin/osascript", "-e", passwordScript, msg).Output()
	if err != nil {
		return "", false
	}
	pw := strings.TrimSuffix(string(out), "\n")
	return pw, pw != ""
}

// notify posts an Alfred notification through the workflow's external
// trigger, falling back to a plain macOS notification.
func notify(title, body string) {
	bundle := env("alfred_workflow_bundleid", defaultBundleID)
	script := `on run argv
	tell application id "com.runningwithcrayons.Alfred" to run trigger (item 1 of argv) in workflow (item 2 of argv) with argument (item 3 of argv)
end run`
	// Alfred never answers if the workflow is not installed, so time out.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script, notifyTrigger, bundle, title+"\n"+body).Run()
	if err != nil {
		fallback := `on run argv
	display notification (item 2 of argv) with title (item 1 of argv)
end run`
		_ = exec.Command("/usr/bin/osascript", "-e", fallback, title, body).Run()
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, body)
}

// notification converts "title\nbody" from the external trigger into
// output for Alfred's Post Notification ({var:notify_title} / {query}).
func notification(arg string) error {
	title, body, _ := strings.Cut(arg, "\n")
	out := map[string]any{"alfredworkflow": map[string]any{
		"arg":       body,
		"variables": map[string]string{"notify_title": title},
	}}
	return json.NewEncoder(os.Stdout).Encode(out)
}
