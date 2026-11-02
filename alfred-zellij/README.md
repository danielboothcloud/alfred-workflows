# Zellij Sessions for Alfred

An Alfred workflow written in Go for managing [Zellij](https://zellij.dev) sessions in [Ghostty](https://ghostty.org).

```
zj            list sessions (running first, then exited)
zj wor        fuzzy filter
zj api        create "api" if it does not exist
zj api @com   create "api" with a layout (built-ins + ~/.config/zellij/layouts)
zj kill all   kill every running session
zj delete all delete every exited session
```

| Key     | Running session                     | Exited session    | “Create …” item          |
|---------|-------------------------------------|-------------------|--------------------------|
| ↩       | focus / switch / attach             | resurrect         | create and attach        |
| ⌥↩      | kill (stays resurrectable)          | delete            | create in the background |
| ⌃↩      | rename (reopens Alfred for the name)|                   |                          |
| fn↩     | open in a new Ghostty window        | same              | same                     |
| ⇧ / ⌘Y  | Quick Look of the active pane       |                   |                          |
| ⌘C / ⌘L | copy name / Large Type              |                   |                          |

No actions use ⌘, because Alfred itself uses ⌘ (⌘1–9, ⌘C, ⌘L, ⌘Y). Deleting a running session takes two steps: ⌥↩ kills it and the list reopens with it marked as exited; ⌥↩ again deletes it.

## How ↩ opens a session

1. **Already on screen**: if a Ghostty terminal is already showing the session, that terminal is focused.
2. **Switch**: otherwise the Zellij client in the frontmost Ghostty terminal is switched to the session with `zellij --session <current> action switch-session <target>`. A switch can also resurrect an exited session or create a new one.
3. **New window**: if no Ghostty terminal is running Zellij, or the switch did not happen, a new Ghostty window runs `zellij attach --create <target>`.

To work out which session each Ghostty terminal shows, the workflow matches terminal ttys from Ghostty's AppleScript to `zellij` client processes (`ps`). It then follows each client's Unix socket to its session server (`lsof`). If a session was renamed and the socket name is stale, it falls back to the terminal title (`<session> | <pane>`).

> **Zellij caveat:** a CLI `switch-session` only moves the session's *last active client*, meaning the last client to send a keystroke. Suppose a client was just switched into a session and nobody has typed in it yet. In that case Zellij ignores the request without reporting an error. The workflow notices this within about a second and opens a new window instead.

Run `make terminals` to see the mapping the workflow uses.

## Install

Requires Go 1.24+, Zellij 0.44+, Ghostty 1.3+ (AppleScript support) and Alfred 5 with the Powerpack.

```sh
make install      # test, build, package and open in Alfred
```

On the first run, macOS asks for permission for Alfred to control Ghostty (Automation). Allow it.

Settings are under *Configure Workflow* in Alfred:

| Variable              | Default   | Meaning                                              |
|-----------------------|-----------|------------------------------------------------------|
| `keyword`             | `zj`      | Keyword                                              |
| `open_mode`           | `window`  | `window` or `tab` for new Ghostty terminals          |
| `default_cwd`         | `~`       | Working directory for new sessions                   |
| `default_layout`      |           | Layout for new sessions when none is picked with `@` |
| `layout_dir`          |           | Extra layouts directory (default `~/.config/zellij/layouts`) |
| `zellij_path`         | auto      | Path to `zellij` if it isn't found automatically     |
| `reopen_after_action` | on        | Reopen the list after kill, delete or rename         |

## Development

```sh
make test         # go vet + unit tests
make build        # workflow/alfred-zellij
make icons        # regenerate PNG icons (tools/genicons)
make universal    # arm64 + amd64 binary
./workflow/alfred-zellij filter "wo" | jq .
```

Layout:

```
main.go                     filter/run/previews/terminals commands, Alfred items
internal/zellij             zellij CLI wrapper and output parsing
internal/ghostty            Ghostty AppleScript: list/focus terminals, open window/tab
internal/attach             terminal → session mapping and the focus/switch/new-window logic
internal/sessions           session details, 2s cache, Quick Look previews
internal/fuzzy              ranking for the filter
internal/alfred             Script Filter JSON
tools/genicons              icon renderer
```

The Script Filter runs only `list-sessions`, `list-clients` and `list-tabs`, about 20ms per zellij process. Results are cached for 2 seconds while you type. Quick Look previews (`dump-screen` of each session's focused pane) are written by a detached background process, so the list never waits for them.
