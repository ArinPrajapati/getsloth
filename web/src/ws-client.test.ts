import { RelayClient, type ConnectionState } from './ws-client';

class FakeSocket {
  static created: FakeSocket[] = [];

  onopen: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;

  readonly sent: string[] = [];
  readonly url: string;

  constructor(url: string) {
    this.url = url;
    FakeSocket.created.push(this);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.onclose?.(new CloseEvent('close'));
  }

  emit(data: unknown): void {
    this.onmessage?.({ data: String(data) } as MessageEvent<string>);
  }
}

describe('RelayClient', () => {
  beforeEach(() => {
    FakeSocket.created = [];
  });

  it('reports connection state changes', () => {
    const states: ConnectionState[] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: (state) => states.push(state),
      onOutput: () => undefined,
      onErrorMessage: () => undefined
    });

    client.connect();
    FakeSocket.created[0]?.onopen?.(new Event('open'));
    FakeSocket.created[0]?.onclose?.(new CloseEvent('close'));

    expect(states).toEqual(['connecting', 'connected', 'disconnected']);
  });

  it('emits decoded terminal bytes from output messages', () => {
    const chunks: number[][] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: () => undefined,
      onOutput: (bytes) => chunks.push([...bytes]),
      onErrorMessage: () => undefined
    });

    client.connect();
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'output', data_base64: 'SGkNCg==' }));

    expect(chunks).toEqual([[72, 105, 13, 10]]);
  });

  it('surfaces relay error messages without throwing', () => {
    const errors: string[] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: () => undefined,
      onOutput: () => undefined,
      onErrorMessage: (message) => errors.push(message)
    });

    client.connect();
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'error', code: 'SESSION_NOT_FOUND', message: 'No session' }));

    expect(errors).toEqual(['No session']);
  });

  it('sends auth messages and reports auth results', () => {
    const authResults: boolean[] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: () => undefined,
      onOutput: () => undefined,
      onErrorMessage: () => undefined,
      onAuthResult: (result) => authResults.push(result.ok)
    });

    client.connect();
    client.sendAuth({ v: 1, type: 'auth', viewer_pubkey_base64: 'pub', ciphertext_base64: 'cipher' });
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'auth_result', ok: true, token: 'token', connection_id: 'viewer-1' }));

    expect(FakeSocket.created[0]?.sent).toEqual([
      JSON.stringify({ v: 1, type: 'auth', viewer_pubkey_base64: 'pub', ciphertext_base64: 'cipher' })
    ]);
    expect(authResults).toEqual([true]);
  });

  it('sends take_control and reports control and presence broadcasts', () => {
    const activeWriters: string[] = [];
    const participantCounts: number[] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: () => undefined,
      onOutput: () => undefined,
      onErrorMessage: () => undefined,
      onControlChanged: (control) => activeWriters.push(control.active_writer_id),
      onPresence: (presence) => participantCounts.push(presence.connections.length)
    });

    client.connect();
    client.sendTakeControl();
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'control_changed', active_writer_id: 'viewer-1', active_writer_role: 'viewer' }));
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'presence', connections: [{ id: 'viewer-1', role: 'viewer', is_active_writer: true }] }));

    expect(FakeSocket.created[0]?.sent).toEqual([JSON.stringify({ v: 1, type: 'take_control' })]);
    expect(activeWriters).toEqual(['viewer-1']);
    expect(participantCounts).toEqual([1]);
  });

  it('sends and receives chat messages separately from terminal output', () => {
    const messages: string[] = [];
    const chunks: number[][] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: () => undefined,
      onOutput: (bytes) => chunks.push([...bytes]),
      onErrorMessage: () => undefined,
      onChatMessage: (message) => messages.push(message.text)
    });

    client.connect();
    client.sendChatMessage('hello host');
    FakeSocket.created[0]?.emit(JSON.stringify({ v: 1, type: 'chat_message', sender_id: 'viewer-1', sender_role: 'viewer', text: 'hello host' }));

    expect(FakeSocket.created[0]?.sent).toEqual([JSON.stringify({ v: 1, type: 'chat_message', text: 'hello host' })]);
    expect(messages).toEqual(['hello host']);
    expect(chunks).toEqual([]);
  });

  it('surfaces transport errors and can disconnect explicitly', () => {
    const states: ConnectionState[] = [];
    const errors: string[] = [];
    const client = new RelayClient({
      url: 'ws://relay.test/ws/viewer/session',
      createSocket: (url) => new FakeSocket(url),
      onStateChange: (state) => states.push(state),
      onOutput: () => undefined,
      onErrorMessage: (message) => errors.push(message)
    });

    client.connect();
    FakeSocket.created[0]?.onerror?.(new Event('error'));
    client.disconnect();

    expect(errors).toEqual(['Connection failed']);
    expect(states).toEqual(['connecting', 'disconnected']);
  });
});
