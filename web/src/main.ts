import { createXtermTerminal } from './xterm-terminal';
import { mountLandingPage } from './landing';
import { mountViewerApp } from './viewer-app';

const root = document.querySelector<HTMLElement>('#app');

if (!root) {
  throw new Error('Missing #app root element');
}

const pageUrl = new URL(window.location.href);

// The root domain (no /s/{session_id}) has no session to join, so it
// renders the marketing landing page instead of an empty viewer shell.
if (!/^\/s\/[^/]+\/?$/.test(pageUrl.pathname)) {
  mountLandingPage(root);
} else {
  // Overridable via VITE_RELAY_BASE_URL (e.g. in a local .env.local, not
  // committed) for pointing a dev server at a local relay instead of
  // production - without this, there was no way to actually test the
  // Frontend against a real relay short of hardcoding a different URL and
  // reverting it before commit.
  const relayBaseUrl = import.meta.env.VITE_RELAY_BASE_URL ?? 'wss://relay.getsloth.dev';

  mountViewerApp(root, {
    pageUrl,
    relayBaseUrl,
    createTerminal: createXtermTerminal
  });
}
