import { mountViewerApp, viewerWebSocketUrl, type ViewerClient, type ViewerClientFactory } from './viewer-app';
import type { AuthMessage } from './auth';
import type { TerminalLike } from './terminal-view';
import type { ConnectionState, RelayClientOptions } from './ws-client';

class FakeTerminal implements TerminalLike {
  readonly writes: Uint8Array[] = [];
  focusCalls = 0;
  size = { cols: 120, rows: 36 };
  viewportSize = { cols: 120, rows: 36 };
  private dataHandler: ((data: string) => void) | null = null;
  private resizeHandler: ((size: { cols: number; rows: number }) => void) | null = null;
  autoFit = false;
  presentationMode = 'fit';

  open(): void {
    return undefined;
  }

  write(data: Uint8Array): void {
    this.writes.push(data);
  }

  onData(handler: (data: string) => void): void {
    this.dataHandler = handler;
  }

  fit(): { cols: number; rows: number } {
    this.size = this.viewportSize;
    return this.viewportSize;
  }

  proposeSize(): { cols: number; rows: number } {
    return this.viewportSize;
  }

  onResize(handler: (size: { cols: number; rows: number }) => void): void {
    this.resizeHandler = handler;
  }

  focus(): void {
    this.focusCalls += 1;
  }

  type(data: string): void {
    this.dataHandler?.(data);
  }

  resize(cols: number, rows: number): void {
    this.size = { cols, rows };
  }

  emitResize(cols: number, rows: number): void {
    this.viewportSize = { cols, rows };
    this.size = this.viewportSize;
    this.resizeHandler?.(this.viewportSize);
  }

  setAutoFit(active: boolean): void {
    this.autoFit = active;
  }

  setPresentationMode(mode: 'fit' | 'actual'): void {
    this.presentationMode = mode;
  }
}

describe('viewerWebSocketUrl', () => {
  it('builds a viewer WebSocket URL from a session page URL', () => {
    const url = viewerWebSocketUrl(new URL('https://getsloth.dev/s/abc123#k=public-key'), 'wss://relay.getsloth.dev');

    expect(url).toBe('wss://relay.getsloth.dev/ws/viewer/abc123');
  });
});

describe('mountViewerApp', () => {
  it('submits encrypted auth and reveals the terminal after auth succeeds', async () => {
    const root = document.createElement('div');
    const sentAuth: AuthMessage[] = [];
    const sentChat: string[] = [];
    const sentInput: number[][] = [];
    const sentResize: Array<{ cols: number; rows: number }> = [];
    const takeControl = vi.fn();
    const client: ViewerClient = {
      connect: vi.fn(),
      disconnect: vi.fn(),
      sendAuth: (message) => {
        sentAuth.push(message);
      },
      sendChatMessage: (text) => {
        sentChat.push(text);
      },
      sendInput: (bytes) => {
        sentInput.push([...bytes]);
      },
      sendResize: (cols, rows) => {
        sentResize.push({ cols, rows });
      },
      sendTakeControl: takeControl
    };
    const capturedOptions: Parameters<ViewerClientFactory>[0][] = [];
    const terminal = new FakeTerminal();

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => terminal,
      createClient: (options) => {
        capturedOptions.push(options);
        return client;
      },
      createAuthMessage: () => Promise.resolve({
        v: 1,
        type: 'auth',
        viewer_pubkey_base64: 'pub',
        ciphertext_base64: 'cipher'
      })
    });

    const password = root.querySelector<HTMLInputElement>('#session-password');
    expect(password).not.toBeNull();

    if (!password) {
      throw new Error('Expected password input to render');
    }

    password.value = 'secret';
    root.querySelector('form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));
    await Promise.resolve();

    expect(sentAuth).toEqual([{ v: 1, type: 'auth', viewer_pubkey_base64: 'pub', ciphertext_base64: 'cipher' }]);
    capturedOptions[0]?.onAuthResult?.({
      v: 1,
      type: 'auth_result',
      ok: true,
      token: 'token',
      connection_id: 'viewer-1',
      mode: 'remote',
      cols: 120,
      rows: 36,
      active_writer_id: 'host-1',
      active_writer_role: 'host'
    });

    expect(root.querySelector('#session-password')).toBeNull();
    expect(root.querySelector<HTMLElement>('[aria-label="Terminal output"]')?.hidden).toBe(false);

    capturedOptions[0]?.onControlChanged?.({ v: 1, type: 'control_changed', active_writer_id: 'viewer-1', active_writer_role: 'viewer', cols: 120, rows: 36 });
    expect(root.querySelector('[aria-label="Control status"]')?.textContent).toContain('You are driving');
    expect(sentResize).toContainEqual({ cols: 120, rows: 36 });

    capturedOptions[0]?.onControlChanged?.({ v: 1, type: 'control_changed', active_writer_id: 'host-1', active_writer_role: 'host', cols: 180, rows: 50 });
    root.querySelector<HTMLButtonElement>('[aria-label="Session control"] button')?.click();
    expect(takeControl).toHaveBeenCalledWith(120, 36);

    // Not the active writer right now (host-1 is) — typed keystrokes must not
    // send, since the relay would silently drop the input anyway.
    terminal.type('y');
    expect(sentInput).toEqual([]);

    const chatInput = root.querySelector<HTMLInputElement>('#chat-message');
    expect(chatInput).not.toBeNull();

    if (!chatInput) {
      throw new Error('Expected chat input to render');
    }

    chatInput.value = 'check auth middleware';
    root.querySelector('[aria-label="Session chat"] form')?.dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));
    capturedOptions[0]?.onChatMessage?.({ v: 1, type: 'chat_message', sender_id: 'viewer-1', sender_role: 'viewer', sender_display_name: 'Phone', text: 'check auth middleware' });

    expect(sentChat).toEqual(['check auth middleware']);
    expect(root.querySelector('[aria-label="Chat messages"]')?.textContent).toContain('check auth middleware');

    // Regain control, then typed keystrokes should send again.
    capturedOptions[0]?.onControlChanged?.({ v: 1, type: 'control_changed', active_writer_id: 'viewer-1', active_writer_role: 'viewer', cols: 120, rows: 36 });
    terminal.emitResize(160, 44);
    terminal.type('y');
    expect(sentInput).toEqual([[121]]);
    expect(sentResize).toContainEqual({ cols: 160, rows: 44 });
  });

  it('uses the status bar to open overlays and focuses only after control is confirmed', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const capturedOptions: RelayClientOptions[] = [];
    const takeControl = vi.fn();

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => terminal,
      createClient: (clientOptions) => {
        capturedOptions.push(clientOptions);
        return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: takeControl };
      }
    });

    capturedOptions[0]?.onAuthResult?.({
      v: 1,
      type: 'auth_result',
      ok: true,
      connection_id: 'viewer-1',
      mode: 'remote',
      cols: 180,
      rows: 50,
      active_writer_id: 'host-1',
      active_writer_role: 'host'
    });

    const chatOverlay = root.querySelector<HTMLElement>('[data-panel="chat"]');
    expect(chatOverlay?.hidden).toBe(true);

    root.querySelector<HTMLButtonElement>('[aria-label="Open chat"]')?.click();
    expect(chatOverlay?.hidden).toBe(false);

    root.querySelector<HTMLButtonElement>('[aria-label="Focus terminal input"]')?.click();
    expect(chatOverlay?.hidden).toBe(true);
    expect(takeControl).toHaveBeenCalledWith(120, 36);
    expect(terminal.focusCalls).toBe(0);

    capturedOptions[0]?.onControlChanged?.({
      v: 1,
      type: 'control_changed',
      active_writer_id: 'viewer-1',
      active_writer_role: 'viewer',
      cols: 120,
      rows: 36
    });
    expect(terminal.focusCalls).toBe(1);

    root.querySelector<HTMLButtonElement>('[aria-label="Open terminal settings"]')?.click();
    const viewMode = root.querySelector<HTMLSelectElement>('#terminal-view-mode');
    expect(viewMode).not.toBeNull();
    if (viewMode) {
      viewMode.value = 'actual';
      viewMode.dispatchEvent(new Event('change', { bubbles: true }));
    }
    expect(terminal.presentationMode).toBe('actual');
  });

  it('makes group sessions visibly read-only while keeping chat and view settings', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const capturedOptions: RelayClientOptions[] = [];
    const takeControl = vi.fn();

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => terminal,
      createClient: (clientOptions) => {
        capturedOptions.push(clientOptions);
        return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: takeControl };
      }
    });

    capturedOptions[0]?.onAuthResult?.({
      v: 1,
      type: 'auth_result',
      ok: true,
      connection_id: 'viewer-1',
      mode: 'group',
      cols: 180,
      rows: 50,
      active_writer_id: 'host-1',
      active_writer_role: 'host'
    });

    expect(root.querySelector<HTMLButtonElement>('[aria-label="Focus terminal input"]')?.hidden).toBe(true);
    expect(root.querySelector<HTMLButtonElement>('[aria-label="Open session control"]')?.hidden).toBe(true);
    expect(root.querySelector<HTMLButtonElement>('[aria-label="Open chat"]')?.hidden).toBe(false);
    expect(root.querySelector<HTMLButtonElement>('[aria-label="Open terminal settings"]')?.hidden).toBe(false);
    expect(root.querySelector('[aria-label="Active writer status"]')?.textContent).toContain('view only');
    expect(terminal.size).toEqual({ cols: 180, rows: 50 });
    expect(takeControl).not.toHaveBeenCalled();
  });

  it('blurs mobile text entry when a non-typing surface is tapped', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const outsideInput = document.createElement('input');
    document.body.append(outsideInput);

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => terminal,
      createClient: () => ({ connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: vi.fn() })
    });

    outsideInput.focus();
    expect(document.activeElement).toBe(outsideInput);

    root.querySelector<HTMLElement>('[aria-label="Terminal output"]')?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));

    expect(document.activeElement).not.toBe(outsideInput);
    expect(terminal.focusCalls).toBe(0);

    outsideInput.remove();
  });

  it('shows auth failures without revealing the terminal', () => {
    const root = document.createElement('div');
    const capturedOptions: Parameters<ViewerClientFactory>[0][] = [];

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => new FakeTerminal(),
      createClient: (options) => {
        capturedOptions.push(options);
        return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: vi.fn() };
      },
      createAuthMessage: () => Promise.resolve({
        v: 1,
        type: 'auth',
        viewer_pubkey_base64: 'pub',
        ciphertext_base64: 'cipher'
      })
    });

    capturedOptions[0]?.onAuthResult?.({ v: 1, type: 'auth_result', ok: false, code: 'AUTH_FAILED' });

    expect(root.querySelector('[role="alert"]')?.textContent).toBe('Wrong password');
    expect(root.querySelector<HTMLElement>('[aria-label="Terminal output"]')?.hidden).toBe(true);
    expect(root.querySelector('[aria-label="Connection status"]')?.textContent).toBe('Connected');
  });

  it('connects the relay client and writes output bytes to the terminal', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const capturedOptions: RelayClientOptions[] = [];
    const createClient: ViewerClientFactory = (clientOptions) => {
      capturedOptions.push(clientOptions);
      return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: vi.fn() };
    };

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => terminal,
      createClient
    });

    const options = capturedOptions[0];
    expect(options?.url).toBe('wss://relay.getsloth.dev/ws/viewer/abc123');
    options?.onStateChange('connected' satisfies ConnectionState);
    options?.onOutput(new Uint8Array([72, 105]));

    expect(root.querySelector('[aria-label="Connection status"]')?.textContent).toContain('Connected');
    expect(terminal.writes).toEqual([new Uint8Array([72, 105])]);
  });

  it('shows a distinct kicked state and clears stale terminal UI', () => {
    const root = document.createElement('div');
    const disconnect = vi.fn();
    const capturedOptions: RelayClientOptions[] = [];
    const createClient: ViewerClientFactory = (clientOptions) => {
      capturedOptions.push(clientOptions);
      return { connect: vi.fn(), disconnect, sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: vi.fn() };
    };

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => new FakeTerminal(),
      createClient
    });

    capturedOptions[0]?.onKicked?.({ v: 1, type: 'kicked', reason: 'kill_switch' });

    expect(root.querySelector('[aria-label="Terminal output"]')).toBeNull();
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).toContain('Host ended your access');
    expect(disconnect).toHaveBeenCalledTimes(1);
  });

  it('shows a distinct session_ended state and clears stale terminal UI', () => {
    const root = document.createElement('div');
    const disconnect = vi.fn();
    const capturedOptions: RelayClientOptions[] = [];
    const createClient: ViewerClientFactory = (clientOptions) => {
      capturedOptions.push(clientOptions);
      return { connect: vi.fn(), disconnect, sendAuth: vi.fn(), sendChatMessage: vi.fn(), sendInput: vi.fn(), sendResize: vi.fn(), sendTakeControl: vi.fn() };
    };

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => new FakeTerminal(),
      createClient
    });

    capturedOptions[0]?.onSessionEnded?.({ v: 1, type: 'session_ended', reason: 'process_exited' });

    expect(root.querySelector('[aria-label="Terminal output"]')).toBeNull();
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).toContain('Session is over');
    expect(root.querySelector('[aria-label="Session status"]')?.textContent).not.toContain('Host ended your access');
    expect(disconnect).toHaveBeenCalledTimes(1);
  });
});
