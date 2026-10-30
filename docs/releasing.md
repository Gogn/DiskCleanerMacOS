# Building prerelease archives

From a clean, committed checkout, run the release builder with the intended tag:

```sh
./scripts/build-release.sh v0.1.0-beta.1
```

The script creates `dist/v0.1.0-beta.1/` with Apple Silicon (`darwin_arm64`) and Intel (`darwin_amd64`) tarballs plus `SHA256SUMS`. Each archive contains `diskcleaner`, `LICENSE`, and `THIRD_PARTY_NOTICES.txt`; the executable's `-version` value is set from the tag. Verify the archive contents and checksums before attaching them to the matching GitHub release. Rebuild from the final committed source; do not publish trial artifacts made from a working tree.

These prerelease binaries are not Developer ID-signed or notarized. The Apple Silicon build is ad-hoc-signed; the Intel build is unsigned. See [README](../README.md) for download, checksum, and macOS security-prompt guidance.
