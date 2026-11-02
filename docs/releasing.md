# Building prerelease archives

Choose a **new, unpublished tag** and build from its clean, committed checkout. The placeholder below must be replaced with the intended version:

```sh
release_tag="vX.Y.Z-beta.N"
./scripts/build-release.sh "$release_tag"
```

The current release scope is Apple Silicon only. The script creates `dist/<tag>/` with `diskcleaner_<tag>_darwin_arm64.tar.gz` and `SHA256SUMS`. The archive contains `diskcleaner`, `LICENSE` and `THIRD_PARTY_NOTICES.txt`. The executable's `-version` value is set from the tag without its leading `v`. Builds use `CGO_ENABLED=0` and `-trimpath`.

Before publishing:

1. Verify the archive's member list, executable permissions and `arm64` target architecture.
2. Check the native binary's `-version` output and its embedded source revision.
3. From the artifact directory, run `shasum -a 256 -c SHA256SUMS`.
4. Attach the verified archive and checksums to the matching **new** GitHub tag/release, with a source link and accurate validation/signing notes.

Rebuild from the final committed source before uploading; do not publish trial artifacts from a working tree. The script replaces local files in its output directory when rerun. This is not permission to overwrite assets on an already published tag: use a new version for changed source or artifacts.

[`v0.1.0-beta.1`](https://github.com/Gogn/DiskCleanerMacOS/releases/tag/v0.1.0-beta.1) and [`v0.1.0-beta.2`](https://github.com/Gogn/DiskCleanerMacOS/releases/tag/v0.1.0-beta.2) are historical releases with their recorded architectures and signing status documented in [validation status](validation.md#published-prerelease-and-current-source). The current target is Apple Silicon-only `v0.1.0-beta.3`; the next prerelease after it needs a new tag such as `v0.1.0-beta.4`. Future release notes must state the evidence for their own build, not reuse earlier verification as proof. Intel is outside the current release scope.

The build script does not perform Developer ID signing or notarization. See the [README](../README.md) for download, checksum and standard macOS security-prompt guidance. Review the [remaining release gates](validation.md#remaining-release-gates) before claiming platform support or stable-release certification. Documentation entry points are in the [index](README.md).
