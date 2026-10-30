# DiskCleaner

A macOS disk analyzer and cleanup tool with a terminal interface. See what takes up space, review your selection, and move files to Trash or delete them permanently.

## Build and run

Requires Go 1.26 or newer.

```sh
go mod download
go build -o diskcleaner ./cmd/diskcleaner
./diskcleaner
```

The app opens directly in the filesystem browser at your home directory, without scanning or opening the exclusion editor. To start elsewhere:

```sh
./diskcleaner -root "$HOME/Downloads"
```

## Usage

1. Browse to a folder. Optionally set exclusions with `e`; saved patterns and suggestions share one checkbox list.
2. Press `s`, then Enter to scan its contents.
3. Select items with Space. Use `p` for an optional dry run.
4. Press `d`, review the paths and deletion mode, then Enter to confirm.

Trash is the default. Change the mode in settings with `o`.

| Key | Action |
| --- | --- |
| ↑ / ↓ or `k` / `j` | Move through items |
| Enter / → | Open a folder |
| ← / `r` | Go to the parent / enter a path |
| `s` / Space | Scan / select an item |
| `t` | Toggle alphabetical / size sorting (largest first) |
| `x` / `e` | Exclude a folder name / edit exclusion patterns |
| `/` | Filter current names; Enter applies, Esc clears |
| `.` | Show / hide dot-prefixed files and folders (shown by default) |
| ↑ / ↓, Space / Enter in exclusions | Choose / toggle a pattern |
| `a` / Tab in exclusions | Focus Add pattern / switch between input and list |
| `?` / `h` | Open help; Esc or `?` returns to the previous screen |
| Left / Right / Tab, `1`–`6` in help | Switch topics / jump directly; Home / End moves within a topic |
| `o` | Settings; ← / → or Space changes deletion mode |
| `p` / `d` | Preview / confirm cleanup |
| `v` / `l` | Item details / last cleanup report |
| ↑ / ↓ or Page Up / Down | Scroll details, previews, confirmations and reports |
| Esc | Go back or cancel ongoing work |
| `q` / Ctrl+C | Quit from the list; Ctrl+C during work cancels it |

The current folder path is highlighted immediately above the file columns. The path, sorting, exclusion count, selected size, hidden-file status, and action hints stay fixed while the list scrolls. Wide terminals show Name, right-aligned Size, Status, and Usage columns. Below 64 columns, each item uses two rows; the minimum browser size is 32 × 12. Names and paths have no surrounding quotation marks; control characters remain escaped. Long names and paths are shortened; `v` shows complete paths and diagnostics. Usage bars compare each measured item against the largest measured item in the folder; partial values remain marked, and unknown/excluded items have no bar.

Opening a folder does not scan it. Changing folders or exclusions clears sizes and selections. Each successful scan refreshes the list; cleanup clears the results until you scan again.

Sorting defaults to alphabetical order, ignoring case. Press `t` in the file list to switch to size, largest first; press it again to return to alphabetical order. The current mode appears above the list. Equal sizes are ordered alphabetically; unknown and excluded items come last. Partial measurements use their available size and keep their `Partial` label. Before scanning, size mode displays `Size (scan needed)` and keeps alphabetical order. Sorting preserves the highlighted item and selections, and the chosen mode stays active while navigating during the current session.

Press `/` to type a case-insensitive name filter. Enter keeps the filter and returns to navigation; Esc from the browser or filter input clears it. Filtering does not scan, change sizes, or remove selections. Selections hidden by the filter or dot-file toggle are counted above the list and explicitly noted in cleanup confirmation; cleanup still includes every selected path. Opening another folder clears the filter.

Press `.` in the browser to hide or reveal names beginning with a dot. Files are shown by default, and the preference lasts for the current session, including when changing folders. This only changes visibility: scans still include hidden files, and hidden selections remain selected. In a text field, `.` is typed normally.

During a scan, the app shows the current item, elapsed time, and processed/total immediate-item counts, including exclusions and other skipped items. Counts are not a time estimate; no percentage or ETA is shown.

Help has six topics: Quick start, Browsing, Scanning & sizes, Exclusions, Cleanup, and Display & keys. Left/Right or Tab switches topics; `1`–`6` jumps directly. Up/Down or Page Up/Down scrolls, and Home/End goes to the start/end. Help opened from exclusions, scan confirmation, or cleanup opens the relevant topic; leaving help restores the previous screen and selection. In path, pattern, and filter inputs, `?` remains a literal input character (a wildcard in patterns). During an operation, Esc or Ctrl+C cancels it.

An empty exclusion list is highlighted red; exclusions remain optional. Cyan marks headings, a muted slate-blue background marks the cursor, and `[x]` marks selected items. Green marks selections and successful results; yellow marks exclusions or incomplete sizes. Warning colors remain visible on the highlighted row. Permanent-deletion warnings and failed cleanup results are red. Labels and selection markers remain visible without color; run with `NO_COLOR=1` to disable colors.

## Exclusions and sizes

Exclusions match names at any depth within the current folder: `cache`, `*.tmp`, `?ache`. A matching folder excludes its contents. Each folder has its own saved patterns; they are not inherited when navigating. A selected folder containing excluded items cannot be cleaned.

Press `e` to see saved masks and 16 optional suggestions together. The highlighted pattern has a description above the list.

| Purpose | Suggested names |
| --- | --- |
| Dependencies and environments | `node_modules`, `.venv`, `venv`, `vendor` |
| Cloud storage and repository history | `CloudStorage`, `.git` |
| Caches | `__pycache__`, `.cache`, `Caches`, `.gradle` |
| Build output | `.next`, `.build`, `DerivedData`, `target`, `dist`, `build` |

Suggestions are not enabled automatically. These are name patterns at every depth, so broad names such as `build` also match unrelated folders. Excluding `.git`, for example, can block cleanup of a selected project containing that directory. Use suggestions only for content you want to skip; disable the pattern and rescan if you want to inspect or clean it.

Use ↑ / ↓ to choose and Space or Enter to toggle; Page Up/Down moves through longer lists. Press `a` or Tab to enter an additional pattern; Enter adds it, while Tab returns to the list. Unchecking a custom pattern keeps it available until the editor is closed. Changes save immediately for the current folder and clear existing scan results and selections. Esc leaves the editor (or cancels input first).

Sizes represent allocated disk space. Incomplete results are marked `Partial` or `Size unknown`. Scans do not follow symbolic links or cross into other volumes; open a volume to scan it directly.

Permanent deletion shows an estimate; APFS snapshots, clones and hard links can affect the space actually reclaimed.

## Cleanup safety

Only selected items are cleaned, after confirmation. System paths and other protected locations are blocked. A Trash failure never falls back to permanent deletion. The app does not request administrator privileges or send data over the network.

Selected objects are checked again before cleanup. New files inside a selected folder are included. Checks and deletion are not atomic, so avoid cleaning folders that other processes are changing. Cancellation stops further work but does not undo changes already made; the report shows each item's outcome.

Settings are saved automatically to `~/Library/Application Support/DiskCleaner/config.json`. If access is denied, you can grant your terminal Full Disk Access in System Settings → Privacy & Security.

## Development

Development build, tested on macOS 27.0.1 with Apple Silicon. macOS 26 compatibility and Intel runtime support still need verification.

```sh
go test ./...
go test -race ./...
go vet ./...
```

See the [design](docs/superpowers/specs/2026-10-30-disk-cleaner-design.md) for detailed behavior and protection rules, and [validation notes](docs/validation.md) for test coverage and remaining release checks.

## License

[GPL-3.0-only](LICENSE). Dependency licenses are in [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt).
