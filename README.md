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

1. Browse to a folder. Optionally set global exclusions with `e`; saved patterns and suggestions share one checkbox list.
2. Press `s`, then Enter to scan its contents.
3. Select items with Space. Use `p` for an optional dry run.
4. Press `d`, review the paths and deletion mode, then Enter to confirm.

Trash is the default. Change the mode in settings with `o`.

| Key | Action |
| --- | --- |
| ↑ / ↓ or `k` / `j` | Move one visible item |
| Page Up / Page Down | Move one visible viewport, accounting for the current header and one- or two-line entries |
| Home / End | Move to the first / last visible item |
| Enter / → | Open a folder |
| ← / `r` | Go to the parent / enter a path |
| `s` / Space | Scan / select an item |
| `t` | Toggle alphabetical / size sorting (largest first) |
| `E` (Shift+E) | Toggle exclusion of the highlighted file or folder name |
| `x` / `e` | Toggle a folder name exclusion / edit exclusion patterns |
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

The current folder path stays directly above the file columns. The compact header has no separate title row and omits `0 selected`. It combines item counts, sorting, global exclusions and visibility where space allows; selections, hidden-selection warnings, selected size, filter input and diagnostics take priority in a tight window. `v` shows full paths, diagnostics and scan status. Wide windows use file columns; below 64 columns, each entry uses two lines. The minimum size is 32 × 12, with room for at least one complete entry even with a filter, selection and message. Names and paths have no surrounding quotation marks; control characters remain escaped. Usage bars compare each measured item against the largest measured item in the folder; partial values stay marked, and unknown/excluded items have no bar.

The browser footer has two fixed lines: navigation first, then actions for the current scan/selection state. Navigation hints stay the same while selections change; `? help` stays available. Commands are included as complete key/action pairs, with less important hints omitted at narrow widths. A selection keeps preview and cleanup hints visible. Filter input instead shows Enter to apply and Esc to clear; `?` is typed literally there.

Opening a folder does not scan it. Successful scans are cached in memory until the app closes. Returning refreshes the immediate listing and restores measurements only when an entry's path, type, identity, modification time and size still match; new, changed, replaced and removed entries do not retain old sizes. A replaced root folder does not inherit the previous root's cache. Nested changes can escape these checks: sizes remain snapshots of the last scan, so rescan when freshness matters. A scan measures each immediate child's total allocated size, including its nested contents; open and scan a child folder to list its own children separately.

Changing folders clears selections and the text filter. The highlighted path is remembered for each visited folder; pressing `t` moves the cursor to the first visible item. Missing or hidden entries fall back to a visible item. Going up highlights the folder just left, even when the app started inside it and its parent has never been visited. Failed navigation leaves the current folder and history intact. Cursor history and scan results are not restored by a new app session.

`s` opens scan confirmation and dismisses earlier browser notifications; Enter starts a fresh scan. A successful rescan replaces only that folder's cached snapshot. A failed or cancelled rescan clears that folder's sizes and selections, preserves other folders' caches, and never publishes partial results. Newly reported errors and cancellation messages remain visible. Changing global exclusions or completing any cleanup attempt clears all cached sizes and scan times; closing cleanup confirmation without starting it keeps them.

After a successful scan, `Last scan` shows its original completion time in local time and counts of unknown/partial measurements across all current entries, including entries hidden by a filter or the dot-file toggle. Excluded and skipped entries are omitted. Returning to cached results preserves the original timestamp; rescanning replaces it. At narrow widths the summary may say `Incomplete`, or move entirely to `v`, which shows the full date, time and counts.

Sorting defaults to alphabetical order, ignoring case. Press `t` in the file list to switch to size, largest first; press it again to return to alphabetical order. Each explicit switch moves the cursor to the first visible item while preserving selections and the current scan. The chosen mode stays active while navigating during the current session. The current mode appears above the list when space allows; `t` remains available when its hint is omitted. Equal sizes are ordered alphabetically; unknown and excluded items come last. Partial measurements use their available size and keep their `Partial` label. Before scanning, size mode displays `Size (scan needed)` and keeps alphabetical order. Automatic sorting during scans, cache restores and navigation keeps the established cursor-history behavior.

Press `/` to type a case-insensitive name filter. Enter keeps the filter and returns to navigation; Esc from the browser or filter input clears it. Filtering does not scan, change sizes, or remove selections. Selections hidden by the filter or dot-file toggle are counted above the list and explicitly noted in cleanup confirmation; cleanup still includes every selected path. Opening another folder clears the filter.

Press `.` in the browser to hide or reveal names beginning with a dot. Files are shown by default, and the preference lasts for the current session, including when changing folders. This only changes visibility: scans still include hidden files, and hidden selections remain selected. In a text field, `.` is typed normally.

During a scan, the app shows the current item, elapsed time, and processed/total immediate-item counts, including exclusions and other skipped items. Counts are not a time estimate; no percentage or ETA is shown.

Help has six topics: Quick start, Browsing, Scanning & sizes, Exclusions, Cleanup, and Display & keys. Left/Right or Tab switches topics; `1`–`6` jumps directly. Up/Down or Page Up/Down scrolls, and Home/End goes to the start/end. Help opened from exclusions, scan confirmation, or cleanup opens the relevant topic; leaving help restores the previous screen and selection. In path, pattern, and filter inputs, `?` remains a literal input character (a wildcard in patterns). During an operation, Esc or Ctrl+C cancels it.

Help, item details, previews, cleanup confirmations/reports and path input show a `Lines` indicator only when their text overflows; a narrow generic screen may use `More` instead. Scan progress, working screens, scan confirmation and settings have no text-scroll indicator. The file list and exclusion editor keep their separate `Rows` counter.

An empty exclusion list is highlighted red; exclusions remain optional. Cyan marks headings, a muted slate-blue background marks the cursor, and `[x]` marks selected items. Green marks selections and successful results; yellow marks exclusions or incomplete sizes. Warning colors remain visible on the highlighted row. Permanent-deletion warnings and failed cleanup results are red. Labels and selection markers remain visible without color; run with `NO_COLOR=1` to disable colors.

## Exclusions and sizes

One global exclusion list applies below any chosen scan root, including a folder or disk. Patterns match descendant names at any depth: `cache`, `*.tmp`, `?ache`; a matching folder encountered there excludes its contents. Editing exclusions anywhere changes this shared list, which is saved automatically. If saving fails, the rules still apply in the current session, but may be lost after exit; the error is shown. There are no per-folder lists, overrides, or inheritance. Existing per-folder lists are merged on load without exact duplicates. Loading does not rewrite settings; the next normal save writes the unified list, and an empty saved list stays empty. A selected folder containing excluded items cannot be cleaned.

Exclusions do not prevent entering a folder: the chosen root and its ancestors are not matched. For example, after entering a folder named `cache`, a manually started scan measures its children even when `cache` is excluded globally. The pattern skips that folder and its contents when it is encountered below another scan root.

Press `E` (Shift+E) in the browser to immediately exclude the highlighted file or folder by its literal name. Press it again to remove that name exclusion. If another pattern excludes the item (for example, `*.tmp`), use `e` to edit that pattern. Special pattern characters in the name are escaped automatically. The rule applies to matching descendant names at every depth below any chosen scan root, saves immediately, and clears all saved scan results and selections.

Press `e` to see saved masks and 16 optional suggestions together. The highlighted pattern has a description above the list.

| Purpose | Suggested names |
| --- | --- |
| Dependencies and environments | `node_modules`, `.venv`, `venv`, `vendor` |
| Cloud storage and repository history | `CloudStorage`, `.git` |
| Caches | `__pycache__`, `.cache`, `Caches`, `.gradle` |
| Build output | `.next`, `.build`, `DerivedData`, `target`, `dist`, `build` |

Suggestions are not enabled automatically. These are name patterns at every depth, so broad names such as `build` also match unrelated folders. Excluding `.git`, for example, can block cleanup of a selected project containing that directory. Use suggestions only for content you want to skip; disable the pattern and rescan if you want to inspect or clean it.

Use ↑ / ↓ to choose and Space or Enter to toggle; Page Up/Down moves through longer lists. Press `a` or Tab to enter an additional pattern; Enter adds it, while Tab returns to the list. Unchecking a custom pattern keeps it available until the editor is closed. Changes update the global list immediately and clear all saved scan results and selections. Esc leaves the editor (or cancels input first).

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
