import { mountViewerApp, viewerWebSocketUrl, type ViewerClient, type ViewerClientFactory } from './viewer-app';
import type { AuthMessage } from './auth';
import type { TerminalLike } from './terminal-view';
import type { ConnectionState, RelayClientOptions } from './ws-client';

class FakeTerminal implements TerminalLike {
  readonly writes: Uint8Array[] = [];

  open(): void {
    return undefined;
  }

  write(data: Uint8Array): void {
    this.writes.push(data);
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
    const takeControl = vi.fn();
    const client: ViewerClient = {
      connect: vi.fn(),
      disconnect: vi.fn(),
      sendAuth: (message) => {
        sentAuth.push(message);
      },
      sendTakeControl: takeControl
    };
    const capturedOptions: Parameters<ViewerClientFactory>[0][] = [];

    mountViewerApp(root, {
      pageUrl: new URL('https://getsloth.dev/s/abc123#k=public-key'),
      relayBaseUrl: 'wss://relay.getsloth.dev',
      createTerminal: () => new FakeTerminal(),
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
    capturedOptions[0]?.onAuthResult?.({ v: 1, type: 'auth_result', ok: true, token: 'token', connection_id: 'viewer-1' });

    expect(root.querySelector('form')).toBeNull();
    expect(root.querySelector<HTMLElement>('[aria-label="Terminal output"]')?.hidden).toBe(false);

    capturedOptions[0]?.onControlChanged?.({ v: 1, type: 'control_changed', active_writer_id: 'viewer-1', active_writer_role: 'viewer' });
    expect(root.querySelector('[aria-label="Control status"]')?.textContent).toContain('You are driving');

    capturedOptions[0]?.onControlChanged?.({ v: 1, type: 'control_changed', active_writer_id: 'host-1', active_writer_role: 'host' });
    root.querySelector<HTMLButtonElement>('[aria-label="Session control"] button')?.click();
    expect(takeControl).toHaveBeenCalledTimes(1);
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
        return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendTakeControl: vi.fn() };
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
  });

  it('connects the relay client and writes output bytes to the terminal', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const capturedOptions: RelayClientOptions[] = [];
    const createClient: ViewerClientFactory = (clientOptions) => {
      capturedOptions.push(clientOptions);
      return { connect: vi.fn(), disconnect: vi.fn(), sendAuth: vi.fn(), sendTakeControl: vi.fn() };
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
});
