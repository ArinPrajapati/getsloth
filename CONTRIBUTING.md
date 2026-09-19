# Contributing to Sloth

Thanks for taking a look. This is a small, early project - here's what
you need to know to send a useful PR.

## Finding something to work on

Check the [open issues](https://github.com/ArinPrajapati/getsloth/issues),
especially ones labeled
[`good first issue`](https://github.com/ArinPrajapati/getsloth/labels/good%20first%20issue)
or [`help wanted`](https://github.com/ArinPrajapati/getsloth/labels/help%20wanted).
If you want to work on something not already filed, open an issue
first and say what you're planning - especially for anything that
touches `docs/protocol.md` (the wire contract between the host CLI,
relay, and browser viewer) or the crypto/auth flow. Small fixes and
typos don't need an issue first.

## Project layout

```
cmd/getsloth/        host CLI - wraps a command in a PTY, talks to the relay
cmd/getsloth-relay/  relay server - brokers host<->viewer WebSocket connections
internal/hostauth/   host-side password verification, key generation
internal/protocol/   shared Go wire-message types
internal/relay/      relay server implementation
web/                 browser viewer (TypeScript, no framework, Vite)
docs/protocol.md     the WebSocket wire protocol - source of truth
CONSTRAINTS.md       this project's quality bar (coverage, lint, security)
```

## Building and testing

Backend (Go):

```
go build ./...
go test ./cmd/... ./internal/... -race
bash scripts/check.sh   # gofmt, vet, staticcheck, gitleaks
```

Frontend (TypeScript, in `web/`):

```
npm install
npm run check   # tsc + eslint
npm test -- --run   # vitest, with coverage
```

Both sides need to stay green before a PR is reviewable. `CONSTRAINTS.md`
documents the exact quality bar (coverage thresholds, what blocks vs.
warns) and why each check exists.

## Making changes

- **Write tests first where you can.** This codebase is built
  test-first throughout (RED, then GREEN, then check for regressions)
  - not a hard rule for every PR, but the existing test files are the
  best reference for the style expected.
- **`docs/protocol.md` changes first, code second**, for anything that
  touches the wire protocol. Both the Go and TypeScript sides implement
  independently against that document - it has to stay the single
  source of truth or the two sides drift.
- **Keep PRs small and focused.** One logical change per PR is easier
  to review and easier to revert if something's wrong.
- **No secrets, ever.** `gitleaks` runs in CI and locally via
  `scripts/check.sh` - if it flags something, don't suppress it,
  actually remove the secret (and rotate it if it was real).

## Commit messages

Explain the *why*, not just the *what* - the diff already shows what
changed. A short first line, then a paragraph of context if the change
isn't self-explanatory (a bug it fixes, a decision it reflects, a
tradeoff it's making).

## License

Sloth is AGPL-3.0 (see [`LICENSE`](LICENSE)). By contributing, you
agree your contribution is licensed under the same terms.
