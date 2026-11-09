# Mole for Alfred

An Alfred workflow written in Go for [Mole](https://mole.fit), the Mac cleaner. It lists your apps so you can uninstall one with Mole, and runs `mo clean`. Both run **unattended in the background**, and the result arrives as a notification.

```
mole           Mole menu: Open Mole, Clean, Uninstall an app…, Last run log
uninstall      every app Mole can uninstall, A–Z, with sizes
uninstall sla  fuzzy search ("vsc" finds Visual Studio Code)
```

### `mole`

| Item              | ↩                                | ⌥↩ (dry run, Ghostty) | ⌘↩                                      |
| ----------------- | -------------------------------- | --------------------- | --------------------------------------- |
| Open Mole         | `mo` in Ghostty                  |                       |                                         |
| Clean             | `mo clean` in the background     | `mo clean --dry-run`  |                                         |
| Uninstall an app… | opens the `uninstall` list       |                       | `mo uninstall` (Mole's picker, Ghostty) |
| Last run log      | opens the last background output |                       |                                         |

### `uninstall`

| Key     | Action                                                  |
| ------- | ------------------------------------------------------- |
| ↩       | `mo uninstall <app>` in the background, no confirmation |
| ⌥↩      | `mo uninstall --dry-run <app>` in Ghostty (preview)     |
| ⌘↩      | reveal in Finder                                        |
| ⌘C / ⌘L | copy path / Large Type                                  |
| ⇧ / ⌘Y  | Quick Look                                              |

⌥↩ is always a dry run. Dry runs and Mole's own menus open in Ghostty, since they are there to be read.

## Background runs

Mole only accepts a confirmation from a real terminal. It refuses a piped "yes", so that nothing gets uninstalled without review. Picking the app in Alfred is the review, so `alfred-mole work` runs Mole in a pseudo-terminal (`internal/runner`) and answers its prompts:

| Mole prompt                                      | Answer                                              |
| ------------------------------------------------ | --------------------------------------------------- |
| `Remove 1 app, …  Enter confirm, ESC cancel`     | Enter                                               |
| `System caches need sudo. Enter continue, Space skip` (clean) | Space, or Enter if *System caches* is enabled |
| `Press Enter to continue` (first-run permissions) | Enter                                              |
| sudo `Password:`                                 | a native password dialog; Cancel stops Mole         |

- You get a notification when the run starts ("Uninstalling Zed…") and another with Mole's summary ("Uninstalled Zed · Removed 1 app · 112MB") or its error.
- Removed files go to the Trash. Personal data (containers, app scripts) is kept, as in Mole's default.
- Runs queue up, one at a time (a lock in the cache folder), and are stopped after 30 minutes.
- The full output of the last run is in `last-run.log` in the workflow cache folder (*mole › Last run log*).
- Notifications go through the workflow's `notify` External Trigger, so they show the workflow icon. If Alfred cannot be reached, a plain macOS notification is shown instead.

## How apps are listed

Apps come from `mo uninstall --list`, so the list matches exactly what Mole will uninstall. This includes protected-app rules, PlayCover apps, input methods and `/Volumes/*/Applications`. That scan takes about 12 seconds, so the result is cached in Alfred's workflow cache folder (`apps.json`):

- The first time, the list shows *Loading apps from Mole…* while a background process runs the scan. Alfred reruns the filter until the list is ready.
- After that, the cached list appears at once. It is refreshed in the background when `/Applications`, `~/Applications` (and a few related folders) change, or when it is more than 6 hours old.
- Apps whose bundle no longer exists are hidden immediately, so an app you just uninstalled disappears before the refresh finishes.

### Which name is passed to `mo uninstall`

`mo uninstall NAME` picks the **first** app whose display name or `.app` folder name equals `NAME` (ignoring case). If none is equal, it falls back to a substring match, which can select several apps. The `uninstall_name` that `--list` reports for Homebrew casks (for example `alfred` for *Alfred 5*) is a cask token, not an app name, so it would go through that substring match.

So the workflow passes the display name, or failing that the `.app` folder name, as long as no app earlier in Mole's list has the same name. If both names are taken, ↩ opens Mole's own picker instead. Run `make list` to see the name chosen for each app.

## Install

Requires Go 1.24+, [Mole](https://github.com/tw93/Mole) (`brew install mole`), Ghostty 1.3+ and Alfred 5 with the Powerpack.

```sh
make install      # test, build, package and open in Alfred
```

Settings are under *Configure Workflow* in Alfred:

| Variable            | Default     | Meaning                                    |
| ------------------- | ----------- | ------------------------------------------ |
| `mole_keyword`      | `mole`      | Keyword for the Mole menu                  |
| `uninstall_keyword` | `uninstall` | Keyword for the app list                   |
| `open_mode`         | `window`    | Ghostty `window` or `tab` (dry runs, Open Mole) |
| `clean_admin`       | off         | Let `mo clean` ask for your password to clean system caches |
| `mole_path`         |             | Path to `mo` (found automatically if empty) |

## Development

```sh
make test         # go vet + unit tests
make build        # workflow/alfred-mole
make refresh      # run mo uninstall --list and rebuild the cache
make list         # print the cached apps and the name passed to mole
make icons        # regenerate workflow/icon.png (tools/genicons)
./workflow/alfred-mole apps "zed" | jq .
```

Layout:

```
main.go                 menu/apps Script Filters, run actions, ranking
background.go           unattended worker, password dialog, notifications
internal/runner         run Mole in a pty, answer its prompts, parse the summary
internal/mole           finding mo, --list parsing and cache, exact target names
internal/ghostty        open a command in a Ghostty window or tab
internal/fuzzy          fuzzy scoring (shared with app-launcher, alfred-zellij)
internal/alfred         Script Filter JSON
tools/genicons          icon renderer
```
