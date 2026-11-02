// Package sessions gathers zellij session details for the Script Filter.
package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"alfred-zellij/internal/zellij"
)

// Info describes one session.
type Info struct {
	zellij.Session
	Clients     int      `json:"clients"`
	Tabs        []string `json:"tabs"`
	ActiveTab   string   `json:"active_tab"`
	PreviewPath string   `json:"preview_path"`
}

type cacheFile struct {
	Time     time.Time `json:"time"`
	Sessions []Info    `json:"sessions"`
}

// CacheTTL bounds how long details are reused while the user types.
const CacheTTL = 2 * time.Second

func cachePath(cacheDir string) string { return filepath.Join(cacheDir, "sessions.json") }

// Invalidate drops cached details after a session was changed.
func Invalidate(cacheDir string) { _ = os.Remove(cachePath(cacheDir)) }

// Load returns session details, reusing a recent cache when available.
// fresh reports whether zellij was queried (rather than the cache used).
func Load(ctx context.Context, z *zellij.CLI, cacheDir string) (infos []Info, fresh bool, err error) {
	if data, err := os.ReadFile(cachePath(cacheDir)); err == nil {
		var c cacheFile
		if json.Unmarshal(data, &c) == nil && time.Since(c.Time) < CacheTTL {
			return c.Sessions, false, nil
		}
	}
	infos, err = Gather(ctx, z, cacheDir)
	if err != nil {
		return nil, false, err
	}
	if data, err := json.Marshal(cacheFile{Time: time.Now(), Sessions: infos}); err == nil {
		tmp := cachePath(cacheDir) + ".tmp"
		if os.WriteFile(tmp, data, 0o600) == nil {
			_ = os.Rename(tmp, cachePath(cacheDir))
		}
	}
	return infos, true, nil
}

// Gather queries zellij for every session. Each zellij invocation costs
// ~20ms of process start-up, so only clients and tabs are fetched here;
// previews are written separately by WritePreviews.
func Gather(ctx context.Context, z *zellij.CLI, cacheDir string) ([]Info, error) {
	list, err := z.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	infos := make([]Info, len(list))
	var wg sync.WaitGroup
	for i, s := range list {
		infos[i] = Info{Session: s}
		if s.Exited {
			continue
		}
		infos[i].PreviewPath = PreviewPath(cacheDir, s.Name)
		wg.Add(2)
		go func(info *Info) {
			defer wg.Done()
			info.Clients, _ = z.ListClients(ctx, info.Name)
		}(&infos[i])
		go func(info *Info) {
			defer wg.Done()
			tabs, _ := z.ListTabs(ctx, info.Name)
			for _, t := range tabs {
				info.Tabs = append(info.Tabs, t.Name)
				if t.Active && info.ActiveTab == "" {
					info.ActiveTab = t.Name
				}
			}
		}(&infos[i])
	}
	wg.Wait()
	return infos, nil
}

// PreviewPath is where the Quick Look preview of a session is written.
func PreviewPath(cacheDir, session string) string {
	return filepath.Join(cacheDir, "previews", fileName(session)+".txt")
}

// WritePreviews writes a text snapshot of the active pane of each running
// session for Quick Look.
func WritePreviews(ctx context.Context, z *zellij.CLI, cacheDir string, infos []Info) {
	_ = os.MkdirAll(filepath.Join(cacheDir, "previews"), 0o700)
	var wg sync.WaitGroup
	for _, info := range infos {
		if info.Exited {
			continue
		}
		wg.Add(1)
		go func(info Info) {
			defer wg.Done()
			writePreview(ctx, z, cacheDir, info)
		}(info)
	}
	wg.Wait()
}

func writePreview(ctx context.Context, z *zellij.CLI, cacheDir string, info Info) {
	var tabs []zellij.Tab
	var panes []zellij.Pane
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); tabs, _ = z.ListTabs(ctx, info.Name) }()
	go func() { defer wg.Done(); panes, _ = z.ListPanes(ctx, info.Name) }()
	wg.Wait()
	pane, ok := zellij.PreviewPane(tabs, panes)
	text := ""
	if ok {
		screen, err := z.DumpScreen(ctx, info.Name, pane.ID)
		if err != nil {
			return
		}
		text = previewText(info, pane.Title, pane.TabName, screen)
	} else {
		text = previewText(info, "", "", "(no terminal pane to preview)")
	}
	// Write atomically so Quick Look never shows a half-written file.
	path := PreviewPath(cacheDir, info.Name)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, []byte(text), 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

func previewText(info Info, paneTitle, tabName, screen string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\n", info.Name)
	if len(info.Tabs) > 0 {
		fmt.Fprintf(&b, "Tabs:    %s\n", strings.Join(info.Tabs, ", "))
	}
	if paneTitle != "" {
		fmt.Fprintf(&b, "Pane:    %s (tab %s)\n", paneTitle, tabName)
	}
	b.WriteString(strings.Repeat("─", 72) + "\n")
	b.WriteString(trimTrailingBlankLines(zellij.StripANSI(screen)))
	b.WriteString("\n")
	return b.String()
}

func trimTrailingBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// fileName makes a session name safe for use as a file name.
func fileName(name string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	sum := sha256.Sum256([]byte(name))
	return safe + "-" + hex.EncodeToString(sum[:4])
}

// RunningNames returns the names of running sessions.
func RunningNames(infos []Info) []string {
	var out []string
	for _, i := range infos {
		if !i.Exited {
			out = append(out, i.Name)
		}
	}
	return out
}
