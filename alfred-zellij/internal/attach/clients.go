// Package attach finds which Ghostty terminal shows which zellij session and
// opens sessions by focusing, switching, or launching terminals.
package attach

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Client is a zellij client process running in a terminal.
type Client struct {
	PID     int
	TTY     string // /dev/ttysNNN
	Session string // "" while disconnected (e.g. mid-switch) or unknown
}

// Clients maps zellij client processes to the sessions they are connected
// to. A client connects to the session socket
// $TMPDIR/zellij-<uid>/<contract>/<session>. lsof shows the server end with
// that path and the client end as "->addr" of the server end.
//
// running holds the current session names. lsof reports the path the socket
// was bound with, which goes stale when a session is renamed; such sessions
// map to "" and callers fall back to terminal titles.
func Clients(ctx context.Context, running []string) ([]Client, error) {
	procs, err := clientProcesses(ctx)
	if err != nil || len(procs) == 0 {
		return procs, err
	}
	sockets, err := lsofUnixSockets(ctx)
	if err != nil {
		return procs, err
	}
	known := map[string]bool{}
	for _, s := range running {
		known[s] = true
	}
	addrSession := map[string]string{}
	for _, fds := range sockets {
		for _, fd := range fds {
			if strings.HasPrefix(fd.name, "/") && strings.Contains(fd.name, "/zellij-") {
				name := filepath.Base(fd.name)
				if known[name] {
					addrSession[fd.addr] = name
				} else {
					addrSession[fd.addr] = ""
				}
			}
		}
	}
	for i := range procs {
		for _, fd := range sockets[procs[i].PID] {
			if peer, ok := strings.CutPrefix(fd.name, "->"); ok {
				if s := addrSession[peer]; s != "" {
					procs[i].Session = s
					break
				}
			}
		}
	}
	return procs, nil
}

// clientProcesses lists zellij processes attached to a tty, excluding servers.
func clientProcesses(ctx context.Context) ([]Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,tty=,args=").Output()
	if err != nil {
		return nil, err
	}
	var clients []Client
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || f[1] == "??" || f[1] == "-" {
			continue
		}
		// Login shells prefix argv[0] with "-", e.g. "-/opt/homebrew/bin/zellij".
		if filepath.Base(strings.TrimPrefix(f[2], "-")) != "zellij" {
			continue
		}
		if isNonClient(f[3:]) {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		clients = append(clients, Client{PID: pid, TTY: "/dev/" + f[1]})
	}
	return clients, sc.Err()
}

// isNonClient reports zellij invocations that are not interactive clients.
func isNonClient(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-s", "--session", "-l", "--layout", "-n", "--new-session-with-layout", "-c", "--config",
			"--config-dir", "--data-dir", "--max-panes", "--layout-string":
			i++ // skip the flag's value
			continue
		}
		switch a {
		case "--server", "action", "ac", "list-sessions", "ls", "kill-session", "k",
			"delete-session", "d", "kill-all-sessions", "ka", "delete-all-sessions", "da",
			"pipe", "run", "r", "plugin", "p", "edit", "e", "setup", "web", "list-aliases", "la":
			return true
		}
		if !strings.HasPrefix(a, "-") {
			// The first positional argument decides the subcommand.
			return false
		}
	}
	return false
}

type socketFD struct {
	addr string // socket address (lsof "d" field)
	name string // bound path or "->peer"
}

// lsofUnixSockets returns unix sockets held by zellij processes, keyed by pid.
func lsofUnixSockets(ctx context.Context) (map[int][]socketFD, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// lsof exits 1 when nothing matches; output is still valid.
	out, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-U", "-a", "-c", "zellij", "-F", "pdn").Output()
	return parseLsof(string(out)), nil
}

func parseLsof(out string) map[int][]socketFD {
	res := map[int][]socketFD{}
	pid := 0
	var cur socketFD
	flush := func() {
		if pid != 0 && cur.name != "" {
			res[pid] = append(res[pid], cur)
		}
		cur = socketFD{}
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		v := line[1:]
		switch line[0] {
		case 'p':
			flush()
			pid, _ = strconv.Atoi(v)
		case 'f':
			flush()
		case 'd':
			cur.addr = v
		case 'n':
			cur.name = v
		}
	}
	flush()
	return res
}

// SessionFromTitle extracts the session from a zellij terminal title. zellij
// titles host terminals "<session> | <pane title>" or just "<session>".
func SessionFromTitle(title string, running []string) string {
	best := ""
	for _, s := range running {
		if (title == s || strings.HasPrefix(title, s+" | ")) && len(s) > len(best) {
			best = s
		}
	}
	return best
}
