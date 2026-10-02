# Accessibility and Performance Audit

Date: 2026-10-01

Issue: #5

## Scope

This audit covers the landing page (`/`) and a real local session viewer
(`/s/<ephemeral-session>`). Changes cover the landing page, shared styles,
auth gate, chat panel, terminal view, and the viewer overlay container.
All browser measurements were
run in headless Chrome on the development machine.

## Changes

- Added a landing-page title and meta description, named section landmarks,
  decorative-device exclusions, and explicit image dimensions.
- Kept the desktop screenshot lazy because it follows the primary copy on the
  mobile viewport; added aspect-ratio reservations so lazy loading does not
  shift the page.
- Added visible `:focus-visible` treatment and reduced-motion handling to the
  shared stylesheet. Landing buttons now meet a 44px minimum target height.
- Named auth and chat headings, associated auth descriptions/errors, exposed
  auth invalid state, announced new chat messages politely, and labeled the
  terminal output region without making its high-volume stream a live region.
- Removed a prohibited ARIA label from the viewer's plain layout container;
  its child overlays retain their accessible names.

## Environment and Commands

- Node `v22.23.1`, npm `10.9.8`
- Lighthouse `13.5.0`
- axe-core CLI `4.13.0`
- Chrome for Testing `154.0.8037.0`, installed outside the repository
- Landing dev server: `http://127.0.0.1:4173`
- Viewer dev server: `http://127.0.0.1:4174`
- Production preview servers: `4175` (landing) and `4176` (viewer)
- Local relay: `127.0.0.1:18080`

Commands run:

```text
npm run check
npm test -- --run
npm run build
./scripts/check.sh
npm audit --audit-level=high
axe <landing-or-viewer-url> --tags wcag2a,wcag2aa,wcag21aa
lighthouse <landing-or-viewer-url> --output=json
```

`npm run check`, `npm run build`, and `./scripts/check.sh` passed. The web test
suite passed with 90/90 tests and reported 95.68% statements, 88.80% branches,
96.00% functions, and 95.68% lines overall. `npm audit --audit-level=high`
reported three moderate development-dependency findings in Vitest and no high
or critical findings. Addressing them requires a separate dependency upgrade
and lockfile update.

## Axe Results

The command used the required WCAG tags: `wcag2a,wcag2aa,wcag21aa`.

| Page | Server | Result |
| --- | --- | --- |
| Landing `/` | Vite dev, port 4173 | 0 violations |
| Landing `/` | Vite production preview, port 4175 | 0 violations |
| Authenticated real session `/s/<ephemeral-session>` | Vite dev, port 4174 | 0 violations |
| Authenticated real session `/s/<ephemeral-session>` | Vite production preview, port 4176 | 0 violations |

The coordinator removed the invalid `aria-label` from the plain
`.viewer-overlays` `div` in `web/src/app.ts`. The final saved axe JSON results
contain zero violations for all four audits, including zero critical or serious
findings on the authenticated viewer.

The authenticated viewer audit used a fresh ephemeral local group session. Its
password, key, and session identifier are intentionally not recorded here.

## Lighthouse Results

Metrics below are from production previews, which exercise the built assets
without Vite's development module graph and throttled dev-server overhead.

| Page | LCP | CLS | Performance | Verdict |
| --- | ---: | ---: | ---: | --- |
| Landing `/`, port 4175 | 2056.9 ms | 0.0109 | 0.99 | Passes thresholds |
| Real session `/s/<ephemeral-session>`, port 4176 | 1577.3 ms | 0 | 0.99 | Passes thresholds |

The landing development-server baseline before the image reservations was
LCP `7555.2 ms` and CLS `0.2321`. Explicit intrinsic dimensions and reserved
aspect-ratio boxes reduced CLS to `0.0109`. In production-preview trials,
deferring the below-copy desktop screenshot brought LCP below the 2500 ms
threshold. Development and production LCP values are not directly comparable.

For transparency, the same Lighthouse run against Vite development servers
reported landing LCP `7569.5 ms` and viewer LCP `6679.0 ms`, with CLS `0.0109`
and `0` respectively. The dev bundle statically loads the full xterm and
viewer graph even on `/`; the production preview is the meaningful shipped
asset measurement, while the dev-server result remains an issue to resolve if
the gate is required specifically against Vite dev mode.

One follow-up performance opportunity is to replace `web/src/main.ts`'s
unconditional static imports of the landing, viewer, and xterm
modules with route-specific dynamic imports. The root route should not fetch
the session viewer and xterm graph before rendering landing content.

The production Lighthouse values above were retained from the prior production
preview audit. The coordinator's change removes an ARIA attribute only, so it
does not affect layout or loading behavior. For the authenticated re-audit, the
ignored preview artifact was rebuilt with
`VITE_RELAY_BASE_URL=ws://127.0.0.1:18080` so the real local session could
connect; no source or configuration file was changed.

## Limitations

- No real phone or real mobile network test was performed.
- Headless Chrome ran on loopback; results do not represent a deployed HTTPS
  origin, CDN cache, or a low-end phone.
- The real viewer used an ephemeral local group session. Its invite password,
  key, and session identifier are intentionally not recorded here.
- axe detects only automated rule coverage; keyboard traversal, screen-reader
  behavior, color perception, and touch ergonomics still need human testing.

## Acceptance Criteria

- Landing axe: met, 0 critical/serious findings.
- Real viewer axe: met, 0 critical/serious findings after the coordinator's
  `.viewer-overlays` fix.
- Production landing Lighthouse: met, LCP <= 2500 ms and CLS <= 0.1.
- Production real viewer Lighthouse: met, LCP <= 2500 ms and CLS <= 0.1.
- Findings written up: met in this document.
