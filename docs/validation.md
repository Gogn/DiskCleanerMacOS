# Implementation and validation — 2026-10-30

The MVP is implemented. The [current design specification](superpowers/specs/2026-10-30-disk-cleaner-design.md) is the behavior authority; the deferred tree-navigation document is not part of this release. The binaries are development builds, not a claim of completed release certification.

## Current behavior and verification update — 2026-10-30

This section is authoritative for the current checkout. Sections under “Historical validation snapshots” below describe earlier stages; their counts and interface descriptions are not current. The final “Remaining release gates” section remains current.

- **Global exclusions:** one list applies to every selected folder or disk; patterns match names at any depth. Existing root-keyed configuration is merged without exact duplicates during load, not written until the next save, then stored as the global list. `E` toggles a literal-name exclusion globally; `e` edits the shared list. Regression coverage: `internal/config/settings_test.go`, `internal/ui/state/global_exclusions_test.go`, and `internal/ui/state/quick_exclude_test.go`.
- **Scan messaging and scroll indicators:** entering scan confirmation clears a stale quick-exclusion notification while newly produced failure/cancellation messages remain visible. Help shows `Lines` only when its content overflows. Generic `Lines`/`More` indicators appear only on scrollable screens with overflow; scanning, previewing, cleaning, scan confirmation, and settings do not show them. Regression coverage: `scan_message_test.go`, `help_indicator_test.go`, and `scroll_indicator_test.go`.
- **Session-only size cache:** completed scans are retained in memory by canonical root until exit. Returning refreshes the immediate listing and restores measurements only when root and entry identities/metadata still match; new, replaced, retyped, or changed entries are unknown until rescanned. Navigation restores the prior cursor path where possible (including highlighting the child when returning to its parent), while clearing selection and filter. A failed/cancelled rescan evicts that root's prior snapshot and never publishes partial results. Global-mask edits and any cleanup attempt clear all saved scans. The cache is not written to configuration or disk. Regression coverage: `scan_cache_test.go` and `navigation_history_test.go`.
- **Last scan status:** after a successful scan with a timestamp, the compact header can show local time and unknown/partial counts over all current immediate entries; excluded and skipped entries are omitted. The full date/time and counts remain in `v` details. Regression coverage: `scan_time_test.go` and `scan_status_test.go`.
- **Compact browser and navigation:** the header adapts to available space; contextual footer hints keep whole key/action tokens and omit lower-priority commands when crowded. `PgUp/PgDn` move by the actual visible viewport; `Home/End` go to the first/last visible item, respecting filters and hidden-file visibility without changing selection or scan state. Pressing `t` explicitly resets the cursor to the first visible item while preserving selections and the scan snapshot; automatic sorting retains cursor-history behavior. Regression coverage: `browser_compact_test.go`, `browser_navigation_test.go`, and `sort_test.go`.
- **Path entry:** `r` opens an empty field for an absolute, `~`/`~/`, or current-folder-relative directory path; `r path` remains visible at widths 32, 40, 64, 80, and 120. Paste or type then press Enter. Matching outer quotes and one copied CRLF newline are handled; spaces and Unicode are accepted, while literal quote/space names remain addressable. Ctrl+U clears only the path field. Multiline, missing, and non-directory paths are rejected without changing browser state; filters and exclusions remain unchanged. Opening a pasted directory does not scan it. Regression coverage: `TestRootInputPastesCopiedPathsWithoutAppendingCurrentFolder` and `TestInvalidRootInputPreservesCurrentBrowserState` in `path_input_test.go`, `browser_compact_test.go`, and the Bubble Tea adapter paste test in `internal/ui/model_test.go`.
- **AI Explain:** the UI flow is `i` on the highlighted item → explicit read-access confirmation → choose `Answer language: English` (default) or `Русский` → Enter to run the installed, authorized Codex CLI (`codex exec`) with read-only command permissions → scrollable informational result. The choice is saved immediately as `explain_language` (`en`/`ru`); it changes only the answer language, while instructions remain in English and no filesystem metadata is added. Config tests cover English defaults for new and legacy settings, round-trip persistence and rejection of invalid values. State tests cover Left/Right/Tab switching, immediate save, language capture at confirmation, unchanged browser state and save failure. Prompt tests verify only the answer-language directive changes and the metadata payload stays the same. The confirmation discloses that Codex may read file contents and files beyond the selected path and send information to Codex; no separate API key is required for an authorized CLI. Read-only command permissions do not guarantee complete isolation from every Codex capability. DiskCleaner does not change selection or start cleanup. Confirmation and results show `Lines` on overflow; the working screen does not. Esc backs out or cancels and waits for active work before returning; errors show diagnostics. Fake-CLI tests cover the invocation contract, output limits/protocol, error classes, timeout, cancellation and child-process cleanup. Metadata regression coverage checks logical and allocated values against filesystem blocks, sparse and empty files, known-zero versus unavailable (`null`) allocation, and directory-entry sizes without recursive totals; the targeted tests and full `internal/explain` suite passed. An offline CLI argument/config parsing probe used a synthetic `CODEX_HOME` and intentionally invalid provider; it made no model request and did not test version detection. No live Codex request was run.

AI Explain PTY smoke checks passed at 80×24 and 32×12 with a fake CLI: confirmation, Enter, scrolling to the result end, Esc back, and quit. At 80×24, a slow fake request was also cancelled with Esc and returned to the browser before quit. Fixtures were disposable and removed; checks used no user file contents, made no model requests, ran no cleanup, and changed no settings.

Current host: macOS 27.0.1, Apple Silicon (`arm64`), Go 1.27.3; the module minimum remains Go 1.26. Current automated evidence after the answer-language and path-entry changes: `go test -race ./...`, `go vet ./...`, and native `go build -o diskcleaner ./cmd/diskcleaner` all passed. This does not certify macOS 26, Intel runtime behavior, or a successful system Trash move.

**Current limitations:** exclusion matching is scoped below the selected scan root; opening an excluded-name folder directly makes it the root, so that root name itself is not excluded. Session-cache validation compares each immediate entry's identity and metadata; changes deeper inside an otherwise unchanged directory may go undetected. `Last scan` reports the latest completed measurement, not real-time filesystem state. See [README: Exclusions and sizes](../README.md#exclusions-and-sizes) for the exclusion boundary and [README: session scan behavior](../README.md#usage) for cache semantics.

PTY smoke checks used 45 disposable files at 80×24 and 32×12. They covered the compact header, Page Down/End/Home, scan and timestamp display, selected state, and complete footer command tokens. These checks performed no cleanup and made no saved-exclusion edits. An earlier terminal walkthrough also checked returning from a child to its parent with cached sizes and using the global exclusions while starting outside the home directory.

## Historical validation snapshots (superseded by the current update above)

The entries below preserve earlier evidence and their original counts; they are not current-suite totals or current UI specifications. In particular, the former per-root exclusion tests/settings described here are superseded by the global-list migration above.

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

This historical check records the prior explicit-sort cursor behavior, superseded by the current reset described above. An interactive sorting check used three disposable files of 4, 16, and 32 KiB. Startup was alphabetical; choosing size mode before scanning applied descending size order when the scan finished. Selecting a file and toggling back preserved that file's cursor and selection. No cleanup operation was executed; the fixture was removed after exiting.

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

## Remaining release gates (current; still incomplete)

1. On macOS 26, run the full suite and manually verify the system `du`, `rm`, `trash`, and built application.
2. Run the Intel binary on an Intel Mac.
3. Verify successful real Trash movement outside the sandbox, with disposable files only.
4. Repeat identity tests on a case-sensitive volume and exercise a real mounted external volume. Current tests cover equivalent spellings on the present volume and injected discovery/device boundaries, not all physical-volume configurations.
5. Run with the minimum Go 1.26 toolchain; the current host supplies Go 1.27.3.

The remaining checks are explicitly incomplete; no macOS 26 or Intel runtime certification is claimed. Filesystem validation and path-based system execution remain non-atomic, as stated in the specification and UI.
