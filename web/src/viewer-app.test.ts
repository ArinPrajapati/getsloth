import { mountViewerApp, viewerWebSocketUrl, type ViewerClientFactory } from './viewer-app';
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
  it('connects the relay client and writes output bytes to the terminal', () => {
    const root = document.createElement('div');
    const terminal = new FakeTerminal();
    const capturedOptions: RelayClientOptions[] = [];
    const createClient: ViewerClientFactory = (clientOptions) => {
      capturedOptions.push(clientOptions);
      return { connect: vi.fn(), disconnect: vi.fn() };
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
