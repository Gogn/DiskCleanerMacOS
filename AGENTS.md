# Project instructions

## Project context

- DiskCleaner is a Go terminal app for macOS. The module minimum is Go 1.26.
- Main areas: `cmd/diskcleaner` starts the app; `internal/config` stores settings; `internal/scan` lists and measures entries; `internal/ignore` handles name patterns; `internal/policy` guards protected paths and identities; `internal/cleanup` previews and executes cleanup; `internal/ui/state` owns UI behavior; `internal/ui/model.go` adapts Bubble Tea messages; `internal/explain` builds metadata and invokes Codex.
- Start with `README.md` and the [documentation index](docs/README.md); detailed user-facing instructions are in [the user guide](docs/usage.md). The [design specification](docs/design.md) is the detailed behavior reference; `docs/validation.md` records current evidence and release gates; `docs/releasing.md` describes packaging.
- Read the relevant package code and tests before changing behavior. Prefer focused changes and keep UI state, filesystem operations, and terminal-adapter behavior in their existing packages.
- Before non-trivial work, consult available project-specific memory and relevant linked notes, if present. Treat memory as context, not authority; verify pending work and time-sensitive claims against the repository. Never copy secrets into memory.

## Safety invariants

- Scanning is explicit. Never start cleanup automatically or treat an explanation as authorization to delete.
- Only manually selected items are cleaned, after confirmation. Preserve protected-path and object-identity checks. Exclusions block matching descendants by default; a selected folder with excluded descendants may be retried only after a separate explicit per-folder confirmation to include all excluded contents. This override is never global or saved, and never bypasses protected-path or identity checks.
- A Trash failure must never fall back to permanent deletion. Preserve per-item reports and cancellation behavior.
- Keep cache invalidation aligned with scan, exclusion, cleanup, and filesystem identity changes. Do not claim cached sizes are real-time measurements.
- AI explanations require an explicit read-access confirmation. The prompt instructions stay in English; `explain_language` selects only the answer language. Keep CLI execution bounded, classify failures without exposing raw logs, and terminate child process groups on cancellation. Read-only command permissions are not a guarantee of complete isolation or a selected-path OS read boundary.
- AI/explain tests should use fake Codex CLIs and synthetic metadata; do not send real user file contents to Codex or make live model requests in tests. Filesystem tests may call real `du` or `rm` only with isolated disposable fixtures. Never run cleanup on user data.

## Development and checks

- For scan-cache iteration, use `./scripts/check.sh quick`. It runs focused tests in `internal/ui/state` matching `SQLite|ScanCache|Cleanup`.
- For changes in other packages, use the focused package test command while iterating.
- For broad changes, before merging, or before a release, run `./scripts/check.sh full`. It runs:

  ```sh
  go test -race ./...
  go vet ./...
  go build -o diskcleaner ./cmd/diskcleaner
  ```

- Do not rerun the full checks for documentation-only or small cosmetic changes unless they affect risk or a release gate.

- Do not claim macOS 26 or native Intel runtime support, notarization, or successful real Trash movement unless those checks were actually performed.
- Keep docs aligned with the current behavior and evidence. Historical validation notes must remain clearly marked; do not turn old test counts or platform checks into current claims.

## Releases

- Build versioned prerelease archives with `./scripts/build-release.sh vMAJOR.MINOR.PATCH[-prerelease]` from a clean committed checkout. The script creates Apple Silicon and Intel tarballs and `SHA256SUMS`; each archive includes `diskcleaner`, `LICENSE`, and `THIRD_PARTY_NOTICES.txt`.
- Verify version output, archive members, and checksums. Rebuild from the final commit before publishing; never publish trial artifacts from a working tree. Do not overwrite assets on a published tag—use a new version/tag instead.
- State signing, notarization, OS, architecture, and Trash-test status precisely. Do not recommend globally disabling Gatekeeper.
- The current GitHub owner/remote is `Gogn`; verify the configured remote and active identity before release actions. Do not hardcode a personal email or store credentials/secrets in the repository.
- Follow Git and publication authorization already provided by the current task and prior conversation. Necessary steps of an explicitly authorized release are covered by that authorization; do not request repeated confirmation. Do not infer release authorization from an ordinary code or documentation task.
