# UX notes — viewer

Working notes for the UX/UI pass, to be done in `worktree/ux-design` once
Backend integration and end-to-end testing are further along. Not a spec,
not binding — just captured thinking so it isn't lost between sessions.

## Why does the host open the link on a second device?

Question raised 2026-09-17. The answer is simple, don't overthink it: he
wants to see his terminal in the browser. That's the whole ask. The reason
he's on a different device doesn't matter — the second he opens the link,
the expectation is his live terminal, in the browser, right there.

So the design bar is just that: does opening the link show the terminal,
clearly and immediately, with everything else (chat, control, quick
actions) in support of that one job rather than competing with it for
attention.

## Reference: YouTube's control bar

Full-screen YouTube shows every control (play, timeline, volume, captions,
settings, fullscreen) at all times — it doesn't hide them behind a gesture
or a menu. But they're a thin overlay on top of the video, not separate
boxes stacked below it competing for space. The video still reads as the
whole screen even though the controls are always present and reachable.

That's the model, not a collapsed/hidden-by-default sheet (an earlier,
wrong instinct in this note's first draft):

- **Terminal = the video.** Full-bleed, the whole screen, the default
  undistracted state.
- **Chat / control / quick-actions = the YouTube control bar.** An overlay
  on top of the terminal, not separate cards stacked in document flow.
  Present and reachable at all times, visually subordinate (thin,
  low-contrast until touched) so they don't fight the terminal for
  attention.
- **The overlay is toggleable.** If the host wants a fully undistracted
  view — just the raw terminal, nothing else on screen — they can hide it,
  same as hiding YouTube's controls.

## Branding stays out of the productivity surface

Branding ("getsloth" wordmark, logo, marketing flourish) belongs on the
landing page and other marketing-facing surfaces — not in the actual
viewer/session screen. The screen the host is using to watch/control a
live session stays free of brand dressing; that's for where we're selling
the product, not where someone's trying to see their terminal.

Revisit this whole note when starting the actual UX pass.
