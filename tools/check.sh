#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
base=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
work=$(mktemp -d "$base/iptest-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
export TMPDIR="$work"
export IPTEST_TEST_TMP="$work/node"
mkdir -p "$IPTEST_TEST_TMP"
go vet ./...
go test -race -count=1 -timeout=120s ./...
npm test
