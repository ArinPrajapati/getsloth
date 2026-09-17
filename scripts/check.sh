#!/usr/bin/env bash
# Fast checks per CONSTRAINTS.md — must stay well under the ~5s edit-loop
# budget for the Go side. Run from the repo root.
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "gofmt: unformatted files:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet ./...
staticcheck ./...
golangci-lint run
gitleaks detect --redact --no-banner --source .

echo "check.sh: all fast checks passed"
