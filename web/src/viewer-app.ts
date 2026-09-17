import { renderAppShell } from './app';
import { createTerminalView, type TerminalLike } from './terminal-view';
import { RelayClient, type ConnectionState, type RelayClientOptions } from './ws-client';

export interface ViewerClient {
  connect(): void;
  disconnect(): void;
}

export type ViewerClientFactory = (options: RelayClientOptions) => ViewerClient;

export interface MountViewerAppOptions {
  pageUrl: URL;
  relayBaseUrl: string;
  createTerminal: () => TerminalLike;
  createClient?: ViewerClientFactory;
}

export function mountViewerApp(root: HTMLElement, options: MountViewerAppOptions): ViewerClient | null {
  renderAppShell(root);

  const terminalElement = root.querySelector<HTMLElement>('[data-terminal]');
  const connectionStatus = root.querySelector<HTMLElement>('[aria-label="Connection status"]');

  if (!terminalElement || !connectionStatus) {
    throw new Error('Viewer shell did not render required regions');
  }

  const terminal = createTerminalView(terminalElement, options.createTerminal);
  const websocketUrl = viewerWebSocketUrl(options.pageUrl, options.relayBaseUrl);

  if (!websocketUrl) {
    connectionStatus.textContent = 'Open a getsloth /s/{session_id} link to connect.';
    return null;
  }

  const createClient = options.createClient ?? ((clientOptions) => new RelayClient(clientOptions));
  const client = createClient({
    url: websocketUrl,
    onStateChange: (state) => {
      connectionStatus.textContent = statusTextFor(state);
    },
    onOutput: (bytes) => {
      terminal.write(bytes);
    },
    onErrorMessage: (message) => {
      connectionStatus.textContent = message;
    }
  });

  client.connect();
  return client;
}

export function viewerWebSocketUrl(pageUrl: URL, relayBaseUrl: string): string | null {
  const match = /^\/s\/([^/]+)\/?$/.exec(pageUrl.pathname);

  if (!match?.[1]) {
    return null;
  }

  const relay = new URL(relayBaseUrl);
  relay.pathname = `/ws/viewer/${encodeURIComponent(match[1])}`;
  relay.search = '';
  relay.hash = '';

  return relay.toString();
}

function statusTextFor(state: ConnectionState): string {
  if (state === 'connecting') {
    return 'Connecting…';
  }

  if (state === 'connected') {
    return 'Connected';
  }

  if (state === 'disconnected') {
    return 'Disconnected';
  }

  return 'Waiting for session';
}
