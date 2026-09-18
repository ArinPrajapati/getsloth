#!/usr/bin/env bash
# Fast checks per CONSTRAINTS.md — must stay well under the ~5s edit-loop
# budget for the Go side. Run from the repo root.
set -euo pipefail

export PATH="$(go env GOPATH)/bin:$PATH"

# Scoped to the actual Go source directories, not a bare "./..." or ".":
# web/node_modules (npm dependencies, gitignored but present after `npm
# install`) can contain stray .go files shipped by some packages (e.g.
# flatted's bundled Go port) that Go's tooling would otherwise pick up,
# since Go doesn't skip node_modules the way JS tooling does.
GO_DIRS="./cmd/... ./internal/..."

unformatted="$(gofmt -l cmd internal)"
if [ -n "$unformatted" ]; then
  echo "gofmt: unformatted files:" >&2
  echo "$unformatted" >&2
  exit 1
fi

go vet $GO_DIRS
staticcheck $GO_DIRS
golangci-lint run ./cmd/... ./internal/...
gitleaks detect --redact --no-banner --source .

echo "check.sh: all fast checks passed"
