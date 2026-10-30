# Historical validation snapshots — 2026-10-30

These dated records preserve earlier checks and their original counts. They are not current-suite totals, current UI specifications, or release certification. For current checkout evidence and remaining gates, see [validation status](../validation.md); for present behavior, see the [user guide](../usage.md) and [design](../design.md).

The entries below preserve earlier evidence and their original counts; they are not current-suite totals or current UI specifications. In particular, the former per-root exclusion tests/settings described here are superseded by the [current global-list migration](../validation.md).

Host: macOS 27.0.1, Apple Silicon; Go 1.27.3. Module minimum remains Go 1.26.

| Check | Result |
| --- | --- |
| `go test ./...` | Pass, 60 top-level tests plus table-driven subtests |
| `go test -race ./...` | Pass |
| `go vet ./...` | Pass |
| `darwin/arm64` build | Pass; version output and interactive startup verified |
| `darwin/amd64` build | Pass; Mach-O x86_64 verified, execution not tested |
| Formatting | `gofmt` applied; final source whitespace checked |
| Case-insensitive and Unicode-equivalent object spellings | Passed on the current filesystem |

Coverage includes literal/wildcard masks, escaped special names, real BSD `du -I` checks, inherited block-size isolation, sparse files, hard links, newline names, symlinks, injected cross-volume scan boundaries, cancellation, complete/partial/unknown measurements, protected paths and identity history, excluded ancestors, unreadable descendants, settings round trips and validation, captured object replacement, optional dry run, cleanup outcomes and cancellation, no Trash fallback, real `rm` on disposable fixtures, UI navigation/invalidation, confirmation, persistence errors, report reopening, narrow-terminal wrapping and scrolling, and Bubble Tea key/paste integration.

An independent source review found two important issues: missing initial volume/Trash identity registration and width-unaware confirmation layout. Both were fixed with regression tests. Associated minor findings were fixed: excluded symlink-target ancestors, report reopening, unknown totals shown without a numeric zero. Additional tests cover system protection rules and the terminal adapter.

A subsequent review identified three more issues, now covered by regression tests:

- Inherited `LC_ALL=C` made BSD `du` interpret Unicode wildcards differently from the cleanup guard. `du` now receives `LC_ALL=en_US.UTF-8`. Tests compare real measurements against Go matching for accented characters, emoji, and combining characters under C, UTF-8, and invalid inherited locales. An end-to-end scan/cleanup test uses a recording executor to verify that excluded nested files block cleanup and nonmatching masks retain their measured size.
- Successful scans now publish their immediate listing together with measurements. Tests cover added and removed entries, changed file types, cursor preservation, an empty directory, and cancellation without publishing partial results.
- List scrolling keeps the current entry visible and diagnostics accessible above the list; the later column-layout update shortens long names and keeps full paths on `v`. Tests cover a 40×24 list and oversized diagnostics in a 40×12 terminal; `v` opens scrollable full paths and messages without changing the selection.

### Initial interface update — 2026-10-30 (historical)

The unified editor and fixed browser layout below supersede the initial layout and suggestion-screen shortcuts in this historical check.

- Startup is explicitly the filesystem browser, with optional exclusions and a shorter key legend. The repeated Trash explanation has been removed from the interface and documentation.
- Empty exclusions are red; cursor/headings, selections, excluded or incomplete entries, deletion warnings, and cleanup outcomes use distinct colors. Text markers remain; `NO_COLOR=1` disables colors. Wrapped lines carry their own color reset, including after scrolling.
- `?` / `h` opens scrollable help and returns to the previous screen and scroll position. Help preserves pending cleanup confirmation and selections. It does not intercept pattern/path input or ongoing operations.
- `e`, then Tab opens opt-in `node_modules` and `CloudStorage` suggestions. Space/Enter toggles the selected pattern through the same validation, persistence, and scan invalidation used by manual exclusions. Esc returns to the editor.
- Regression checks cover immediate file listing, help/confirmation state, literal wildcard input, suggestion persistence and root isolation, and colored/no-color layouts at 40×12. The full suite, race detector, and `go vet` pass; the local `diskcleaner` executable was rebuilt.
- Interactive startup in an 80×24 terminal showed the file list immediately. `?` opened help, returning and pressing `e` → Tab opened suggestions, and quitting restored the terminal. A second run with color enabled verified the red empty-exclusions status and cyan cursor/header; the first run honored inherited `NO_COLOR=1`. These checks did not apply exclusions or perform cleanup.

### Browser improvements — 2026-10-30 (historical)

- Fixed folder/status header and contextual footer; scrolling affects only the item area. Name, right-aligned Size, Status, and relative Usage bars appear in wide windows; below 64 columns, items use two rows. Browser layouts are checked from 32×12 through wider sizes. Full paths and diagnostics remain available with `v`.
- Cursor background is separate from checkboxes; excluded/partial/unknown warnings keep their foreground color even under the cursor. Color and `NO_COLOR` layouts are identical.
- `/` filters current names without changing the scan or selection. Hidden selections are counted in the header and explicitly disclosed in cleanup confirmation. Empty results expose no actionable file. Enter applies, Esc clears, and navigation resets the filter.
- `e` combines saved masks and suggestions in one checkbox list. Space/Enter toggles, `a` or Tab focuses Add pattern, and Enter adds without accidentally disabling an existing mask. Invalid input is retained; unchecked custom masks remain available until closing the editor.
- Scan progress reports the current immediate item, elapsed seconds, and processed/total item counts, including skipped items. A per-operation synchronized mailbox and one UI timer keep worker writes separate from UI state. Counts are not percentages or time estimates. Cancelled work cannot publish sizes or revive progress in a subsequent screen.
- The 56-test suite, full race detector, and static analysis pass. New checks cover filtering/hidden confirmations, unified pattern editing, fixed headers, responsive columns/bars, warning colors on the cursor, progress before measurement, skipped counts, and cancellation. The local executable was rebuilt.
- Interactive checks used disposable fixtures in 80×24 and 40×12 terminals. Verified fixed headers/footers, Unicode name truncation and full-path confirmation, columns and bars after a scan, a hidden selection disclosed by filtering and confirmation, the unified exclusions list and Add pattern focus, and live progress on 883 immediate items (current paths, processed counts, and elapsed seconds). Confirmation was cancelled; no cleanup or saved-exclusion changes were performed. Fixture files were removed after exiting.

### Help, suggestions, and visibility update (historical)

- The exclusion list has 16 opt-in suggestions spanning dependencies, cloud storage, Git metadata, caches, and build output. The active pattern's description stays visible; custom additions receive focus immediately. Descriptions explain broad names rather than implying every match is disposable.
- Help has six topics, context-aware entry, fixed navigation hints, direct keys 1–6, Left/Right/Tab topic changes, and Home/End plus line/page scrolling. Closing help restores the caller's screen and position.
- `.` toggles dot-prefixed entries, shown by default. This session-only preference survives navigation and never changes scan scope, selected targets, or stored exclusions. Selections hidden by either visibility control are counted and explicitly included in confirmation. In text inputs, a dot remains literal.
- Names and paths no longer have enclosing quotes. Newline, tab, ESC, and other control characters remain escaped. The current folder path is colored and fixed immediately above the file columns, including when filters or diagnostics are shown.
- All 60 tests pass, including full race checks; `go vet ./...` passes. Regression coverage includes hidden-only folders, retained hidden selections and exact confirmation targets, topic navigation/return, all suggestions and their descriptions at 32×12, safe escaping without wrappers, and path placement. The local executable was rebuilt.
- Interactive checks in 80×24 and 40×12 terminals verified dot-file visibility, quote-free names, the colored path above columns, the expanded suggestion list with descriptions, contextual help entry, direct topic switching, and word-wrapped help text. No cleanup or exclusion changes were executed.

### Interactive smoke test (historical)

Sorting update: `t` toggles alphabetical (default, case-insensitive) and size (largest first). Tests cover cursor/selection/confirmation preservation, snapshot independence, equal and partial sizes, unknown/excluded entries, invalidation, navigation, empty lists, and automatic ordering after a scan. The full suite, UI race checks, and `go vet ./...` pass; the local executable was rebuilt.

This historical check records the prior explicit-sort cursor behavior, superseded by the [current explicit-sort reset](../usage.md#sorting-filtering-and-hidden-files). An interactive sorting check used three disposable files of 4, 16, and 32 KiB. Startup was alphabetical; choosing size mode before scanning applied descending size order when the scan finished. Selecting a file and toggling back preserved that file's cursor and selection. No cleanup operation was executed; the fixture was removed after exiting.

In an 80-column, 24-row terminal, using only `.work/smoke` fixtures:

- Initial immediate listing appeared without scanning.
- `s` displayed the warning; Enter ran the scan and showed allocated sizes.
- Space selected a file; `p` displayed its read-only dry run.
- `d` displayed the frozen path and Trash mode; Enter invoked the system Trash command.
- The sandbox denied access to Trash. The report showed `Failed — changes may have occurred`, then cleared the snapshot and selection. The fixture remained present, demonstrating no fallback to permanent deletion.
- Esc returned to the unscanned listing; `q` restored the terminal and exited normally.

Successful real Trash movement was **not** verified in this environment. The cancellation and failure paths were verified without touching pre-existing user files. Real permanent deletion is covered by isolated fixture tests.

### Implementation decisions (historical)

- Keep the public `ui.New` Bubble Tea interface, with lifecycle logic in `internal/ui/state` for direct tests. The existing `charmbracelet/x/ansi` dependency is now direct for Unicode cell-width measurement and safe truncation; no new module version was introduced.
- Conservatively reject unresolvable and out-of-root symlink targets. This can block cleaning some harmless links; it avoids treating an unverifiable target as safe.
- Add `l` to reopen the last cleanup report and support scrolling long exclusion lists. These make specified information accessible after leaving its first screen.
- Keep work in the requested project directory. Git initialization was initially blocked; the user initialized the repository and downloaded dependencies. Agent writes to `.git` remain restricted, so no commits, merges, or pushes were performed.
- Keep macOS 26 as an intended target, not verified support. Cross-compilation alone does not validate an OS or hardware target.
