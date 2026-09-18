import { renderAppShell } from './app';
import { createAuthGate } from './auth-gate';
import { createAuthMessage, type AuthMessage, type CreateAuthMessageOptions } from './auth';
import { createChatPanel } from './chat-panel';
import { createControlPanel } from './control-panel';
import { createSessionState } from './session-state';
import { createTerminalView, type TerminalLike, type TerminalPresentationMode, type TerminalSize } from './terminal-view';
import type { SessionMode } from './protocol';
import { RelayClient, type ConnectionState, type RelayClientOptions } from './ws-client';

export interface ViewerClient {
  connect(): void;
  disconnect(): void;
  sendAuth(message: AuthMessage): void;
  sendChatMessage(text: string): void;
  sendInput(bytes: Uint8Array): void;
  sendResize(cols: number, rows: number): void;
  sendTakeControl(cols: number, rows: number): void;
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
  const activeWriterStatus = root.querySelector<HTMLElement>('[aria-label="Active writer status"]');
  const chatOverlay = root.querySelector<HTMLElement>('[data-panel="chat"]');
  const controlOverlay = root.querySelector<HTMLElement>('[data-panel="control"]');
  const statusBar = root.querySelector<HTMLElement>('[aria-label="Session status bar"]');
  const typeButton = root.querySelector<HTMLButtonElement>('[aria-label="Focus terminal input"]');
  const controlButton = root.querySelector<HTMLButtonElement>('[aria-label="Open session control"]');

  if (!terminalCard || !terminalElement || !connectionStatus || !activeWriterStatus || !chatOverlay || !controlOverlay || !statusBar || !typeButton || !controlButton) {
    throw new Error('Viewer shell did not render required regions');
  }

  const activeWriterStatusElement = activeWriterStatus;
  const controlOverlayElement = controlOverlay;
  const typeButtonElement = typeButton;
  const controlButtonElement = controlButton;

  terminalCard.hidden = true;
  const terminal = createTerminalView(terminalElement, options.createTerminal, {
    onInput: (bytes) => {
      client.sendInput(bytes);
    },
    onResize: (size) => {
      client.sendResize(size.cols, size.rows);
    }
  });
  const websocketUrl = viewerWebSocketUrl(options.pageUrl, options.relayBaseUrl);
  const hostPublicKeyBase64Url = hostPublicKeyFromFragment(options.pageUrl);

  if (!websocketUrl) {
    connectionStatus.textContent = 'missing session';
    return null;
  }

  if (!hostPublicKeyBase64Url) {
    connectionStatus.textContent = 'missing auth key';
    return null;
  }

  const createClient = options.createClient ?? ((clientOptions) => new RelayClient(clientOptions));
  const sessionState = createSessionState(root);
  let localConnectionId: string | null = null;
  let sessionMode: SessionMode = 'remote';
  let focusWhenControlArrives = false;
  const chatPanel = createChatPanel(chatOverlay, {
    onSend: (text) => {
      client.sendChatMessage(text);
    }
  });
  const controlPanel = createControlPanel(controlOverlay, {
    localConnectionId,
    onTakeControl: requestControl
  });
  wireStatusBar(root, {
    onType: requestControl,
    onPresentationMode: (mode) => {
      terminal.setPresentationMode(mode);
    }
  });

  function requestControl(): void {
    if (sessionMode === 'group') {
      return;
    }

    const size = terminal.desiredSize();
    focusWhenControlArrives = true;
    activeWriterStatusElement.textContent = 'requesting control…';
    client.sendTakeControl(size.cols, size.rows);
  }

  function applySessionMode(mode: SessionMode): void {
    sessionMode = mode;
    const isGroup = mode === 'group';
    typeButtonElement.hidden = isGroup;
    controlButtonElement.hidden = isGroup;

    if (isGroup) {
      controlOverlayElement.hidden = true;
      activeWriterStatusElement.textContent = 'group · view only';
    }
  }

  function applyControl(activeWriterId: string, activeWriterRole: 'host' | 'viewer', size: TerminalSize): void {
    terminal.setCanonicalSize(size);
    const isActiveWriter = sessionMode === 'remote' && activeWriterId === localConnectionId;
    terminal.setActive(isActiveWriter);
    controlPanel.updateControl({
      active_writer_id: activeWriterId,
      active_writer_role: activeWriterRole
    });

    if (sessionMode === 'group') {
      activeWriterStatusElement.textContent = 'group · view only';
      return;
    }

    activeWriterStatusElement.textContent = isActiveWriter ? 'you driving' : `${roleLabel(activeWriterRole)} driving`;

    if (isActiveWriter && focusWhenControlArrives) {
      focusWhenControlArrives = false;
      terminal.focus();
    }
  }
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
      }).catch((error: unknown) => {
        gate.showError(error instanceof Error ? error.message : 'Could not encrypt password attempt');
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
        applySessionMode(result.mode ?? 'remote');

        if (
          result.cols !== undefined &&
          result.rows !== undefined &&
          result.active_writer_id !== undefined &&
          result.active_writer_role !== undefined
        ) {
          applyControl(result.active_writer_id, result.active_writer_role, { cols: result.cols, rows: result.rows });
        }

        gate.remove();
        terminalCard.hidden = false;
        connectionStatus.textContent = 'Connected';
        return;
      }

      if (result.code === 'RATE_LIMITED') {
        gate.showError('Too many attempts. Try again soon.');
      } else if (result.code === 'SESSION_OCCUPIED') {
        gate.showError('This remote session already has a viewer. Ask the host to use group mode for more viewers.');
      } else {
        gate.showError('Wrong password');
      }
      connectionStatus.textContent = 'Connected';
    },
    onChatMessage: (message) => {
      chatPanel.addMessage(message);
    },
    onControlChanged: (control) => {
      applyControl(control.active_writer_id, control.active_writer_role, { cols: control.cols, rows: control.rows });
    },
    onPresence: (presence) => {
      controlPanel.updatePresence(presence.connections);
    },
    onTerminalSize: (size) => {
      terminal.setCanonicalSize({ cols: size.cols, rows: size.rows });
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

interface StatusBarOptions {
  onType(): void;
  onPresentationMode(mode: TerminalPresentationMode): void;
}

function wireStatusBar(root: HTMLElement, options: StatusBarOptions): void {
  const overlays = [...root.querySelectorAll<HTMLElement>('.viewer-overlay')];
  const buttons = [...root.querySelectorAll<HTMLButtonElement>('.status-bar-button[data-panel]')];

  function showPanel(panelName: string | null): void {
    for (const overlay of overlays) {
      overlay.hidden = overlay.dataset.panel !== panelName;
    }

    for (const button of buttons) {
      const isPressed = button.dataset.panel === panelName;
      button.setAttribute('aria-pressed', String(isPressed));
    }
  }

  function blurTextEntryForNonTypingTap(event: Event): void {
    const target = event.target;

    if (!(target instanceof HTMLElement) || isTextEntryTarget(target) || target.closest('[data-panel="type"]')) {
      return;
    }

    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }
  }

  root.addEventListener('pointerdown', blurTextEntryForNonTypingTap);
  root.addEventListener('mousedown', blurTextEntryForNonTypingTap);

  for (const button of buttons) {
    button.setAttribute('aria-pressed', 'false');
    button.addEventListener('click', () => {
      if (button.dataset.panel === 'type') {
        showPanel(null);
        options.onType();
        return;
      }

      const panelName = button.dataset.panel ?? null;
      const shouldClose = panelName !== null && !root.querySelector<HTMLElement>(`.viewer-overlay[data-panel="${panelName}"]`)?.hidden;
      showPanel(shouldClose ? null : panelName);
    });
  }

  root.querySelector<HTMLSelectElement>('#terminal-view-mode')?.addEventListener('change', (event) => {
    const value = (event.currentTarget as HTMLSelectElement).value;
    options.onPresentationMode(value === 'actual' ? 'actual' : 'fit');
  });
}

function isTextEntryTarget(target: HTMLElement): boolean {
  return target.closest('input, textarea, select, [contenteditable="true"]') !== null;
}

function roleLabel(role: 'host' | 'viewer'): string {
  return role === 'host' ? 'host' : 'viewer';
}
