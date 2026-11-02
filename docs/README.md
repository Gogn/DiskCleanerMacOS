# Documentation

Start with the [project README](../README.md) for downloads, screenshots, building and a quick start. The detailed documents below describe the current source checkout. Beta.2 remains the latest published prerelease; the next release target is `v0.1.0-beta.3`, scoped to Apple Silicon (`arm64`) only. [Validation status](validation.md#published-prerelease-and-current-source) preserves beta.1/beta.2 evidence and distinguishes it from current-source verification.

## Current references

| Document | Purpose |
| --- | --- |
| [User guide](usage.md) | Complete keys, browsing and paths, scans and cache, global exclusions, AI read access, cleanup and display |
| [Design](design.md) | Detailed behavior and protection rules; primarily in Russian |
| [Validation status](validation.md) | Recorded source and published-release evidence, known limitations and remaining release gates |
| [Release guide](releasing.md) | Build and verify archives for a new, unpublished version |

For AI implementation details, the base English instructions and answer-language directive are in [prompt.go](../internal/explain/prompt.go). [explain.go](../internal/explain/explain.go) combines them with the [metadata](../internal/explain/metadata.go) and invokes the [CLI runner](../internal/explain/cli.go). The `en`/`ru` setting changes the answer language, not the language of the instructions. Read-only command permissions are not a selected-path read boundary or a guarantee of complete isolation; see the [user guide](usage.md#ai-explanations).

## Historical records

These preserve earlier plans and evidence, not current requirements or a task queue:

- [Original MVP implementation plan — 2026-10-30](archive/2026-10-30-mvp-plan.md)
- [Historical validation snapshots — 2026-10-30](archive/2026-10-30-validation.md)

## Deferred proposal

[Tree navigation and a separate scan root](future/tree-navigation.md) is a design proposal. It is not implemented, scheduled or an approved implementation queue.
