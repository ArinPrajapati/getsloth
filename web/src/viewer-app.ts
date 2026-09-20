import { renderAppShell } from './app';
import { createAuthGate } from './auth-gate';
import { createAuthMessage, type AuthMessage, type CreateAuthMessageOptions } from './auth';
import { createChatPanel } from './chat-panel';
import { createControlPanel } from './control-panel';
import { mobileTerminalKeyBytes, type MobileTerminalKey } from './mobile-terminal-keys';
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
  const terminalHelper = root.querySelector<HTMLElement>('[aria-label="Terminal helper keys"]');
  const typeButton = root.querySelector<HTMLButtonElement>('[aria-label="Focus terminal input"]');
  const controlButton = root.querySelector<HTMLButtonElement>('[aria-label="Open session control"]');

  if (!terminalCard || !terminalElement || !connectionStatus || !activeWriterStatus || !chatOverlay || !controlOverlay || !statusBar || !terminalHelper || !typeButton || !controlButton) {
    throw new Error('Viewer shell did not render required regions');
  }

  const activeWriterStatusElement = activeWriterStatus;
  const controlOverlayElement = controlOverlay;
  const terminalHelperElement = terminalHelper;
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

  // The QR-only link (cmd/getsloth/shareurl.go's qrShareURL) carries the
  // session password in the fragment so scanning can auto-authenticate -
  // see docs/ideas/getsloth.md's "QR bypasses the password prompt"
  // decision. That's fine for the moment of scanning (whoever sees the QR
  // already sees the password on the terminal), but leaving the plaintext
  // password sitting in the browser's address bar and history afterward
  // is a materially wider, longer-lived exposure than that. Strip it
  // immediately after reading it, before anything else happens with it.
  const passwordFromQR = passwordFromFragment(options.pageUrl);
  if (passwordFromQR !== null) {
    stripPasswordFromAddressBar(hostPublicKeyBase64Url);
  }

  const createClient = options.createClient ?? ((clientOptions) => new RelayClient(clientOptions));
  const sessionState = createSessionState(root);
  let localConnectionId: string | null = null;
  let sessionMode: SessionMode = 'remote';
  let isActiveWriter = false;
  let focusWhenControlArrives = false;
  let controlModifierArmed = false;
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
    onLeaveTyping: leaveTyping,
    onPresentationMode: (mode) => {
      terminal.setPresentationMode(mode);
    }
  });
  wireTerminalHelper(terminalHelperElement, {
    onKey: (key) => {
      terminal.sendInput(mobileTerminalKeyBytes(key));
    },
    onControl: () => {
      controlModifierArmed = !controlModifierArmed;
      terminal.setControlModifier(controlModifierArmed);
      terminalHelperElement.dataset.controlArmed = String(controlModifierArmed);
    },
    onPaste: () => {
      void pasteTerminalInput();
    }
  });

  async function pasteTerminalInput(): Promise<void> {
    try {
      const text = await navigator.clipboard.readText();
      terminal.sendInput(new TextEncoder().encode(text));
    } catch {
      // Clipboard access can be denied by a browser or embedded web view.
      // The visible status preserves typing mode and points to the OS fallback.
      activeWriterStatusElement.textContent = 'paste unavailable';
    }
  }

  function setTyping(active: boolean): void {
    const typing = active && sessionMode === 'remote' && isActiveWriter;
    terminalHelperElement.hidden = !typing;
    if (!typing) {
      controlModifierArmed = false;
      terminal.setControlModifier(false);
      delete terminalHelperElement.dataset.controlArmed;
    }
  }

  function leaveTyping(): void {
    setTyping(false);
  }

  function requestControl(): void {
    if (sessionMode === 'group') {
      return;
    }

    if (isActiveWriter) {
      terminal.focus();
      setTyping(true);
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
      setTyping(false);
    }
  }

  function applyControl(activeWriterId: string, activeWriterRole: 'host' | 'viewer', size: TerminalSize): void {
    terminal.setCanonicalSize(size);
    isActiveWriter = sessionMode === 'remote' && activeWriterId === localConnectionId;
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

    if (!isActiveWriter) {
      setTyping(false);
      return;
    }

    if (focusWhenControlArrives) {
      focusWhenControlArrives = false;
      terminal.focus();
      setTyping(true);
    }
  }
  const gate = createAuthGate(root, {
    initialPassword: passwordFromQR ?? undefined,
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

// Only present when the page was opened from the terminal QR code, per
// qrShareURL in cmd/getsloth/shareurl.go - the plain copy/paste share link
// never carries this.
function passwordFromFragment(pageUrl: URL): string | null {
  const params = new URLSearchParams(pageUrl.hash.replace(/^#/, ''));
  return params.get('p');
}

function sessionIdFromUrl(pageUrl: URL): string | null {
  return /^\/s\/([^/]+)\/?$/.exec(pageUrl.pathname)?.[1] ?? null;
}

// Rewrites the visible address bar to drop the `p=` password param,
// leaving `k=` (and the rest of the URL) untouched, via replaceState so
// it doesn't create a new back-button entry. The fragment was never sent
// over the network either way (see docs/protocol.md), but leaving the
// plaintext password sitting in the browser's own address bar and local
// history for the rest of the session is unnecessary exposure once it's
// already been read - guards against a later reader of that history
// (another person with the device, a synced account, a history-reading
// extension), not against the relay or network.
function stripPasswordFromAddressBar(hostPublicKeyBase64Url: string): void {
  const cleaned = new URL(window.location.href);
  cleaned.hash = `k=${hostPublicKeyBase64Url}`;
  window.history.replaceState(window.history.state as unknown, '', cleaned.toString());
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

interface TerminalHelperOptions {
  onKey(key: MobileTerminalKey): void;
  onControl(): void;
  onPaste(): void;
}

function wireTerminalHelper(helper: HTMLElement, options: TerminalHelperOptions): void {
  for (const button of helper.querySelectorAll<HTMLButtonElement>('[data-terminal-key]')) {
    button.addEventListener('pointerdown', (event) => {
      // A button focus would dismiss the mobile OS keyboard. Keep xterm's
      // textarea focused while still allowing the following click to fire.
      event.preventDefault();
    });
    button.addEventListener('click', () => {
      const key = button.dataset.terminalKey;
      if (key === 'ctrl') {
        options.onControl();
      } else if (key === 'paste') {
        options.onPaste();
      } else if (isMobileTerminalKey(key)) {
        options.onKey(key);
      }
    });
  }
}

function isMobileTerminalKey(key: string | undefined): key is MobileTerminalKey {
  return key === 'escape' || key === 'tab' || key === 'arrow-left' || key === 'arrow-up' || key === 'arrow-down' || key === 'arrow-right';
}

interface StatusBarOptions {
  onType(): void;
  onLeaveTyping(): void;
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

  for (const button of buttons) {
    button.setAttribute('aria-pressed', 'false');
    button.addEventListener('click', () => {
      if (button.dataset.panel === 'type') {
        showPanel(null);
        options.onType();
        return;
      }

      options.onLeaveTyping();
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


function roleLabel(role: 'host' | 'viewer'): string {
  return role === 'host' ? 'host' : 'viewer';
}
