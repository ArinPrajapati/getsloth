import { createXtermTerminal } from './xterm-terminal';
import { mountViewerApp } from './viewer-app';

const root = document.querySelector<HTMLElement>('#app');

if (!root) {
  throw new Error('Missing #app root element');
}

// Overridable via VITE_RELAY_BASE_URL (e.g. in a local .env.local, not
// committed) for pointing a dev server at a local relay instead of
// production - without this, there was no way to actually test the
// Frontend against a real relay short of hardcoding a different URL and
// reverting it before commit.
const relayBaseUrl = import.meta.env.VITE_RELAY_BASE_URL ?? 'wss://relay.getsloth.dev';

mountViewerApp(root, {
  pageUrl: new URL(window.location.href),
  relayBaseUrl,
  createTerminal: createXtermTerminal
});
