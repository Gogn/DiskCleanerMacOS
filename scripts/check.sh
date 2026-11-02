#!/usr/bin/env bash
set -euo pipefail

# quick (default): focused cache and cleanup tests for everyday iteration.
# full: all race tests, vet, and the application build; stops on first failure.
usage() {
  echo "Usage: $0 [quick|full] (default: quick)" >&2
}

if [[ $# -gt 1 ]]; then
  usage
  exit 2
fi

check_mode="${1-quick}"
case "$check_mode" in
  quick|full) ;;
  *) usage; exit 2 ;;
esac

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd -- "$repo_root"

case "$check_mode" in
  quick)
    go test ./internal/ui/state -run 'SQLite|ScanCache|Cleanup'
    ;;
  full)
    go test -race ./...
    go vet ./...
    go build -o diskcleaner ./cmd/diskcleaner
    ;;
esac
