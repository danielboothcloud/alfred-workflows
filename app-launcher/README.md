# App Launcher for Alfred

An Alfred workflow written in Go that finds and opens applications **without relying on Spotlight**. Use it when the Mac's index is broken and apps are missing from Spotlight or Alfred.

```
app            list every app, A–Z
app slack      fuzzy search ("vsc" finds Visual Studio Code)
```

| Key     | Action                     |
| ↩       | open the app               |
| ⌘↩      | reveal in Finder           |
| ⌘C / ⌘L | copy path / Large Type     |
| ⇧ / ⌘Y  | Quick Look                 |
| →       | Alfred's file actions      |

Alfred learns which apps you pick (each result has a stable `uid`), so the apps you open most rise to the top.

## How apps are found

The binary walks these folders, in this order, and goes up to three subfolders deep:

- `/Applications`
- `~/Applications`
- `/System/Applications` (including `Utilities`)
- `/System/Cryptexes/App/System/Applications` (Safari)
- `/System/Library/CoreServices/Applications` (Screen Sharing, Directory Utility, …)
- any folders listed in the `extra_dirs` setting

Finder is added as well. The walk does not look inside `.app` bundles, so helper apps are not listed. It follows symlinked folders such as Nix or Home Manager apps, and it lists a bundle reachable through a symlink once (for example `/Applications/Safari.app`).

Titles are the names Finder shows (`NSFileManager displayNameAtPath:`), so `VoiceMemos.app` appears as “Voice Memos”. Search matches both the display name and the bundle file name.

### Index

A full scan takes about 70ms. The result is cached in Alfred's workflow cache folder (`index.json`), along with the modification time of every folder that was read. On each keystroke the workflow checks those times. If an app was installed, removed or renamed in any searched folder, it rescans. It also rescans at least every 24 hours. To force a rebuild, run `workflow/alfred-apps reindex` (or `make list`).

## Install

Requires Go 1.24+, the Xcode command line tools (cgo with Foundation), and Alfred 5 with the Powerpack.

```sh
make install      # test, build, package and open in Alfred
```

Settings are under *Configure Workflow* in Alfred:

| Variable     | Default | Meaning                                    |
| `keyword`    | `app`   | Keyword                                    |
| `extra_dirs` |         | Extra folders to search, colon separated   |

## Development

```sh
make test         # go vet + unit tests
make build        # workflow/alfred-apps
make list         # rebuild the index and print every app
make icons        # regenerate workflow/icon.png (tools/genicons)
make universal    # arm64 + amd64 binary
./workflow/alfred-apps filter "code" | jq .
```

Layout:

```
main.go                 filter/list/reindex commands, Alfred items, ranking
internal/apps           folder walk, localised names (cgo), cached index
internal/fuzzy          fuzzy scoring (shared with alfred-zellij)
internal/alfred         Script Filter JSON
tools/genicons          icon renderer
```
