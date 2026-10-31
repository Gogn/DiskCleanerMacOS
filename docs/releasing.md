# Building prerelease archives

Choose a **new, unpublished tag** and build from its clean, committed checkout. The placeholder below must be replaced with the intended version:

```sh
release_tag="vX.Y.Z-beta.N"
./scripts/build-release.sh "$release_tag"
```

The script creates `dist/<tag>/` with `diskcleaner_<tag>_darwin_arm64.tar.gz`, `diskcleaner_<tag>_darwin_amd64.tar.gz` and `SHA256SUMS`. Each archive contains `diskcleaner`, `LICENSE` and `THIRD_PARTY_NOTICES.txt`. The executable's `-version` value is set from the tag without its leading `v`. Builds use `CGO_ENABLED=0` and `-trimpath` for both macOS architectures.

Before publishing:

1. Verify both archives' member lists, executable permissions and target architectures.
2. Check the native binary's `-version` output and its embedded source revision. Cross-compilation alone is not native runtime verification.
3. From the artifact directory, run `shasum -a 256 -c SHA256SUMS`.
4. Attach the verified archives and checksums to the matching **new** GitHub tag/release, with a source link and accurate validation/signing notes.

Rebuild from the final committed source before uploading; do not publish trial artifacts from a working tree. The script replaces local files in its output directory when rerun. This is not permission to overwrite assets on an already published tag: use a new version for changed source or artifacts.

[`v0.1.0-beta.1`](https://github.com/Gogn/DiskCleanerMacOS/releases/tag/v0.1.0-beta.1) and [`v0.1.0-beta.2`](https://github.com/Gogn/DiskCleanerMacOS/releases/tag/v0.1.0-beta.2) have already been published; the next prerelease needs a new tag such as `v0.1.0-beta.3`. Their recorded signing status and source revisions are in [validation status](validation.md#published-prerelease-and-current-source). Future release notes must state the evidence for their own build, not reuse earlier verification as proof.

The build script does not perform Developer ID signing or notarization. See the [README](../README.md) for download, checksum and standard macOS security-prompt guidance. Review the [remaining release gates](validation.md#remaining-release-gates) before claiming platform support or stable-release certification. Documentation entry points are in the [index](README.md).
