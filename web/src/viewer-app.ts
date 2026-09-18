import { renderAppShell } from './app';
import { createAuthGate } from './auth-gate';
import { createAuthMessage, type AuthMessage, type CreateAuthMessageOptions } from './auth';
import { createChatPanel } from './chat-panel';
import { createControlPanel } from './control-panel';
import { createQuickActions } from './quick-actions';
import { createSessionState } from './session-state';
import { createTerminalView, type TerminalLike } from './terminal-view';
import { RelayClient, type ConnectionState, type RelayClientOptions } from './ws-client';

export interface ViewerClient {
  connect(): void;
  disconnect(): void;
  sendAuth(message: AuthMessage): void;
  sendChatMessage(text: string): void;
  sendInput(bytes: Uint8Array): void;
  sendTakeControl(): void;
}

export type ViewerClientFactory = (options: RelayClientOptions) => ViewerClient;

export interface MountViewerAppOptions {
  pageUrl: URL;
  relayBaseUrl: string;
  createTerminal: () => TerminalLike;
  createClient?: ViewerClientFactory;
  createAuthMessage?: (options: CreateAuthMessageOptions) => Promise<AuthMessage>;
}

export function mountViewerApp(root: HTMLElement, options: MountViewerAppOptions): ViewerClient | null {
  renderAppShell(root);

  const terminalCard = root.querySelector<HTMLElement>('[aria-label="Terminal output"]');
  const terminalElement = root.querySelector<HTMLElement>('[data-terminal]');
  const connectionStatus = root.querySelector<HTMLElement>('[aria-label="Connection status"]');

  if (!terminalCard || !terminalElement || !connectionStatus) {
    throw new Error('Viewer shell did not render required regions');
  }

  terminalCard.hidden = true;
  const terminal = createTerminalView(terminalElement, options.createTerminal, {
    onInput: (bytes) => {
      client.sendInput(bytes);
    }
  });
  const websocketUrl = viewerWebSocketUrl(options.pageUrl, options.relayBaseUrl);
  const hostPublicKeyBase64Url = hostPublicKeyFromFragment(options.pageUrl);

  if (!websocketUrl) {
    connectionStatus.textContent = 'Open a getsloth /s/{session_id} link to connect.';
    return null;
  }

  if (!hostPublicKeyBase64Url) {
    connectionStatus.textContent = 'Share link is missing the host auth key.';
    return null;
  }

  const createClient = options.createClient ?? ((clientOptions) => new RelayClient(clientOptions));
  const sessionState = createSessionState(root);
  let localConnectionId: string | null = null;
  const chatPanel = createChatPanel(root, {
    onSend: (text) => {
      client.sendChatMessage(text);
    }
  });
  const quickActions = createQuickActions(root, {
    onInput: (bytes) => {
      client.sendInput(bytes);
    }
  });
  const controlPanel = createControlPanel(root, {
    localConnectionId,
    onTakeControl: () => {
      client.sendTakeControl();
    }
  });
  const gate = createAuthGate(root, {
    onSubmit: (submission) => {
      connectionStatus.textContent = 'Checking password…';
      void (options.createAuthMessage ?? createAuthMessage)({
        sessionId: sessionIdFromUrl(options.pageUrl) ?? '',
        hostPublicKeyBase64Url,
        password: submission.password,
        displayName: submission.displayName
      }).then((message) => {
        client.sendAuth(message);
      }).catch(() => {
        gate.showError('Could not encrypt password attempt');
      });
    }
  });
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
    },
    onAuthResult: (result) => {
      if (result.ok) {
        localConnectionId = result.connection_id ?? null;
        controlPanel.setLocalConnectionId(localConnectionId);
        gate.remove();
        terminalCard.hidden = false;
        connectionStatus.textContent = 'Connected';
        return;
      }

      gate.showError(result.code === 'RATE_LIMITED' ? 'Too many attempts. Try again soon.' : 'Wrong password');
    },
    onChatMessage: (message) => {
      chatPanel.addMessage(message);
    },
    onControlChanged: (control) => {
      controlPanel.updateControl(control);
      const isActiveWriter = control.active_writer_id === localConnectionId;
      quickActions.setActive(isActiveWriter);
      terminal.setActive(isActiveWriter);
    },
    onPresence: (presence) => {
      controlPanel.updatePresence(presence.connections);
    },
    onKicked: () => {
      client.disconnect();
      sessionState.showKicked();
    },
    onSessionEnded: (message) => {
      client.disconnect();
      sessionState.showSessionEnded(message.reason);
    }
  });

  client.connect();
  return client;
}

export function viewerWebSocketUrl(pageUrl: URL, relayBaseUrl: string): string | null {
  const sessionId = sessionIdFromUrl(pageUrl);

  if (!sessionId) {
    return null;
  }

  const relay = new URL(relayBaseUrl);
  relay.pathname = `/ws/viewer/${encodeURIComponent(sessionId)}`;
  relay.search = '';
  relay.hash = '';

  return relay.toString();
}

function hostPublicKeyFromFragment(pageUrl: URL): string | null {
  const params = new URLSearchParams(pageUrl.hash.replace(/^#/, ''));
  return params.get('k');
}

function sessionIdFromUrl(pageUrl: URL): string | null {
  return /^\/s\/([^/]+)\/?$/.exec(pageUrl.pathname)?.[1] ?? null;
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
