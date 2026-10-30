#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$ ]]; then
  echo "Usage: $0 vMAJOR.MINOR.PATCH[-prerelease]" >&2
  exit 1
fi

release_tag="$1"
release_version="${release_tag#v}"
repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
output_dir="$repo_root/dist/$release_tag"
mkdir -p -- "$output_dir"
stage_dir="$(mktemp -d "$output_dir/.build.XXXXXX")"
trap 'rm -rf -- "$stage_dir"' EXIT

# Do not store the builder's account name or extended metadata in archives.
tar_version="$(tar --version)"
case "$tar_version" in
  bsdtar*) tar_identity=(--uid 0 --gid 0 --uname root --gname root) ;;
  *GNU*) tar_identity=(--owner=0 --group=0 --numeric-owner) ;;
  *) echo "Unsupported tar implementation" >&2; exit 1 ;;
esac

cd -- "$repo_root"
archives=()
for release_arch in arm64 amd64; do
  mkdir -p -- "$stage_dir/$release_arch"
  CGO_ENABLED=0 GOOS=darwin GOARCH="$release_arch" \
    go build -trimpath \
      -ldflags "-s -w -X main.buildVersion=$release_version" \
      -o "$stage_dir/$release_arch/diskcleaner" ./cmd/diskcleaner
  chmod 0755 "$stage_dir/$release_arch/diskcleaner"
  cp "$repo_root/LICENSE" "$repo_root/THIRD_PARTY_NOTICES.txt" "$stage_dir/$release_arch/"
  chmod 0644 "$stage_dir/$release_arch/LICENSE" "$stage_dir/$release_arch/THIRD_PARTY_NOTICES.txt"
  archive="diskcleaner_${release_tag}_darwin_${release_arch}.tar.gz"
  COPYFILE_DISABLE=1 tar -czf "$stage_dir/$archive" --format ustar \
    "${tar_identity[@]}" -C "$stage_dir/$release_arch" diskcleaner LICENSE THIRD_PARTY_NOTICES.txt
  archives+=("$archive")
done

(
  cd -- "$stage_dir"
  shasum -a 256 "${archives[@]}" > SHA256SUMS
)
for archive in "${archives[@]}" SHA256SUMS; do
  mv -f -- "$stage_dir/$archive" "$output_dir/$archive"
done
printf 'Release artifacts: %s\n' "$output_dir"
