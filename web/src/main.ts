import { createXtermTerminal } from './xterm-terminal';
import { mountViewerApp } from './viewer-app';

const root = document.querySelector<HTMLElement>('#app');

if (!root) {
  throw new Error('Missing #app root element');
}

mountViewerApp(root, {
  pageUrl: new URL(window.location.href),
  relayBaseUrl: 'wss://relay.getsloth.dev',
  createTerminal: createXtermTerminal
});
