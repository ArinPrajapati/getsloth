# Constraints

Last reviewed: 2026-09-17

Project: getsloth (v0). Greenfield, no code written yet — this file is binding
from the first line, not retrofitted after the fact. Stack: Go for the CLI +
relay server, TypeScript + xterm.js for the browser client. Most code is
AI-generated and human-reviewed; the explicit goal of this file is to prevent
that code from reading as low-effort ("AI slop") to an open-source audience.

## Floor (always enforced, no setup required)

- No new suppression comments: `//nolint`, `// eslint-disable`, `@ts-ignore`
- No unimplemented stubs: `panic("not implemented")`, `TODO` standing in for
  real logic, empty `catch {}` / empty `if err != nil {}` swallowing errors
- No skipped or deleted tests without a reason in the commit message
- No secrets in source
- No comments that narrate what the code does ("// increment counter") —
  comments only for non-obvious *why* (a workaround, a hidden constraint, a
  surprising invariant)
- This file does not get weakened to make a change pass

## Enforced with numbers

| Dimension | Rule | Checked by | Runs at |
|---|---|---|---|
| Go format | Zero unformatted files | `gofmt -l .` | every edit |
| Go vet | Zero findings | `go vet ./...` | every edit |
| Go lint | Zero errors from config | `staticcheck ./...` | every edit |
| TS types | Zero type errors | `tsc --noEmit` | every edit |
| JS/TS lint | Zero errors from config | `eslint .` | every edit |
| Secrets | No secrets in source | `gitleaks detect --redact --no-banner` | every edit |
| Coverage (Go) | Touched packages ≥ 80% | `go test ./... -cover` (per touched package) | task end |
| Coverage (TS) | Changed lines ≥ 80% | `vitest run --coverage` + git diff | task end |
| Security: Go static | Zero high findings | `gosec ./...` | task end, CI |
| Security: Go deps | Nothing at high or above | `govulncheck ./...` | CI |
| Security: JS deps | Nothing at high or above | `npm audit --audit-level=high` | CI |
| Architecture (Go) | Relay package never imports password-comparison/auth-decision logic | `golangci-lint run` (depguard rule) | CI |
| Accessibility | Zero critical or serious | `axe http://localhost:PORT --tags wcag2a,wcag2aa,wcag21aa` | before ship |
| Performance | LCP ≤ 2500ms, CLS ≤ 0.1 on the web viewer | `lighthouse http://localhost:PORT --output=json` | before ship |

Every row names the command that produces the verdict. A dimension with a
number and no command in this column is an aspiration, not a constraint.

**Why these numbers:** 80% changed-line/package coverage is high enough to
force a real test on new logic (password verification, control handoff,
rate limiting) without demanding tests for trivial glue code. Accessibility
and performance are checked before ship, not on every edit, because they
need a running server — they're real requirements given the headline pitch
is mobile use, but too slow for the edit loop.

**Architecture boundary, stated explicitly:** the relay server must never
contain password-comparison or auth-decision logic — that lives only in the
host CLI process. This is the mechanism behind the "relay never learns the
password" claim in the spec; a `depguard` rule makes it impossible to
accidentally violate, not just a promise in prose.

## Exceptions

| ID | Rule | Path | Reason | Owner | Expires |
|---|---|---|---|---|---|
| — | none yet | — | — | — | — |

## Notes on enforcement

- Floor + secrets + the architecture boundary rule **block**, always — these
  are the non-negotiables given the explicit "must not read as slop, must not
  leak the password" goals.
- Coverage, gosec/govulncheck/npm-audit findings, accessibility, and
  performance **warn** for now (first two weeks / through the Friday launch),
  and become blocking once there's enough real code and CI history to hold a
  line against. Warnings are still visible in every review pass, not silent.
- Fast checks (format, vet, lint, types, secrets) run on every edit and must
  stay under ~5 seconds. Coverage and static-security checks run when a task
  is considered done, budgeted at ~90 seconds. Accessibility, performance,
  dependency scans, and the architecture lint run in CI / before ship —
  they're real gates, just not in the edit loop.
