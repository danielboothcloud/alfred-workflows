package attach

import (
	"context"
	"fmt"
	"log"
	"time"

	"alfred-zellij/internal/ghostty"
	"alfred-zellij/internal/zellij"
)

// View is a Ghostty terminal that shows a zellij client.
type View struct {
	Terminal ghostty.Terminal
	Client   Client
}

// Views pairs Ghostty terminals with the zellij clients on their ttys.
// Sessions come from the client's socket, falling back to the terminal
// title when the socket name is stale (renamed session).
func Views(ctx context.Context, terms []ghostty.Terminal, running []string) ([]View, error) {
	return views(ctx, terms, running, true)
}

func views(ctx context.Context, terms []ghostty.Terminal, running []string, titleFallback bool) ([]View, error) {
	clients, err := Clients(ctx, running)
	if err != nil {
		return nil, err
	}
	byTTY := map[string]Client{}
	for _, c := range clients {
		if _, dup := byTTY[c.TTY]; !dup || c.Session != "" {
			byTTY[c.TTY] = c
		}
	}
	var views []View
	for _, t := range terms {
		c, ok := byTTY[t.TTY]
		if !ok {
			continue
		}
		if c.Session == "" && titleFallback {
			c.Session = SessionFromTitle(t.Title, running)
		}
		views = append(views, View{Terminal: t, Client: c})
	}
	return views, nil
}

// Options control how a session is opened.
type Options struct {
	NewWindow bool   // skip focusing/switching and always open a new terminal
	Exists    bool   // target is listed by zellij (running or exited)
	Layout    string // layout for a new session
	Cwd       string // working directory for a new session
	Mode      string // "window" or "tab" for new Ghostty terminals
}

// Opener opens zellij sessions in Ghostty.
type Opener struct {
	Z       *zellij.CLI
	Running []string // names of running sessions
	Logf    func(format string, args ...any)
}

func (o *Opener) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// Open shows target in Ghostty, trying in order:
//  1. focus a terminal already showing target;
//  2. switch the zellij client in the frontmost Ghostty terminal (or any
//     Ghostty terminal running zellij) to target and focus it;
//  3. open a new Ghostty window running `zellij attach --create target`.
func (o *Opener) Open(ctx context.Context, target string, opts Options) error {
	if !opts.NewWindow && ghostty.Running(ctx) {
		done, err := o.reuse(ctx, target, opts)
		if err != nil {
			o.logf("reuse terminal: %v", err)
		}
		if done {
			return nil
		}
	}
	cwd := ""
	if !opts.Exists {
		cwd = opts.Cwd
	}
	cmd := o.Z.AttachCommand(target, opts.Layout, opts.Exists)
	o.logf("opening new ghostty %s: %s", opts.Mode, cmd)
	return ghostty.Open(ctx, cmd, cwd, opts.Mode)
}

func (o *Opener) reuse(ctx context.Context, target string, opts Options) (bool, error) {
	terms, err := ghostty.Terminals(ctx)
	if err != nil {
		return false, err
	}
	views, err := Views(ctx, terms, o.Running)
	if err != nil {
		return false, err
	}
	for _, v := range views {
		if v.Client.Session == target {
			o.logf("%q already shown in terminal %s", target, v.Terminal.ID)
			return true, ghostty.Focus(ctx, v.Terminal.ID)
		}
	}
	host, ok := pickHost(views)
	if !ok {
		o.logf("no ghostty terminal is running a zellij client")
		return false, nil
	}

	sw := zellij.SwitchOptions{}
	if !opts.Exists {
		sw = zellij.SwitchOptions{Layout: opts.Layout, Cwd: opts.Cwd}
	}
	o.logf("switching client of %q (terminal %s) to %q", host.Client.Session, host.Terminal.ID, target)
	if err := o.Z.SwitchSession(ctx, host.Client.Session, target, sw); err != nil {
		return false, err
	}

	before := map[int]string{}
	for _, v := range views {
		before[v.Client.PID] = v.Client.Session
	}
	v, ok := o.waitForSwitch(ctx, terms, before, host.Client.Session, target)
	if !ok {
		o.logf("switch did not happen (no client in %q has typed since joining it)", host.Client.Session)
		return false, nil
	}
	return true, ghostty.Focus(ctx, v.Terminal.ID)
}

// pickHost prefers the focused terminal of the front Ghostty window.
func pickHost(views []View) (View, bool) {
	var fallback *View
	for i, v := range views {
		if v.Client.Session == "" {
			continue
		}
		if v.Terminal.Focused {
			return v, true
		}
		if fallback == nil {
			fallback = &views[i]
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return View{}, false
}

// waitForSwitch polls until a client from host lands in target. zellij
// switches the host's last active client, which may not be the terminal we
// picked, so every client of host is watched. If none has left host shortly
// after the request, the switch was ignored.
func (o *Opener) waitForSwitch(ctx context.Context, terms []ghostty.Terminal, before map[int]string, host, target string) (View, bool) {
	const (
		interval     = 100 * time.Millisecond
		ignoredAfter = 800 * time.Millisecond
		giveUpAfter  = 6 * time.Second
	)
	running := append(append([]string{}, o.Running...), target)
	start := time.Now()
	for {
		time.Sleep(interval)
		// Socket data only: a reconnecting client has no socket, and its
		// title still names the old session, which would hide the move.
		current, err := views(ctx, terms, running, false)
		if err != nil {
			return View{}, false
		}
		moving := false
		for _, v := range current {
			was, tracked := before[v.Client.PID]
			if !tracked || was != host {
				continue
			}
			if v.Client.Session == target {
				return v, true
			}
			if v.Client.Session != host {
				moving = true // disconnected from host, still connecting
			}
		}
		elapsed := time.Since(start)
		if (!moving && elapsed > ignoredAfter) || elapsed > giveUpAfter {
			return View{}, false
		}
	}
}

// Describe returns a human-readable summary of Ghostty terminals and the
// zellij sessions they show (for debugging).
func Describe(ctx context.Context, running []string) (string, error) {
	if !ghostty.Running(ctx) {
		return "Ghostty is not running", nil
	}
	terms, err := ghostty.Terminals(ctx)
	if err != nil {
		return "", err
	}
	views, err := Views(ctx, terms, running)
	if err != nil {
		return "", err
	}
	byID := map[string]View{}
	for _, v := range views {
		byID[v.Terminal.ID] = v
	}
	out := ""
	for _, t := range terms {
		session := "-"
		if v, ok := byID[t.ID]; ok {
			session = fmt.Sprintf("%q (pid %d)", v.Client.Session, v.Client.PID)
		}
		focus := " "
		if t.Focused {
			focus = "*"
		}
		out += fmt.Sprintf("%s %s %-14s session=%s title=%q\n", focus, t.ID, t.TTY, session, t.Title)
	}
	return out, nil
}
