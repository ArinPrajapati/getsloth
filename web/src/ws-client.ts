import type { AuthMessage } from './auth';
import {
  decodeBase64Bytes,
  parseRelayMessage,
  type AuthResultMsg,
  type ControlChangedMsg,
  type PresenceMsg
} from './protocol';

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'disconnected';

export interface SocketLike {
  onopen: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<string>) => void) | null;
  send(data: string): void;
  close(): void;
}

export interface RelayClientOptions {
  url: string;
  createSocket?: (url: string) => SocketLike;
  onStateChange(state: ConnectionState): void;
  onOutput(bytes: Uint8Array): void;
  onErrorMessage(message: string): void;
  onAuthResult?(result: AuthResultMsg): void;
  onControlChanged?(control: ControlChangedMsg): void;
  onPresence?(presence: PresenceMsg): void;
}

export class RelayClient {
  private readonly createSocket: (url: string) => SocketLike;
  private readonly onAuthResult: (result: AuthResultMsg) => void;
  private readonly onControlChanged: (control: ControlChangedMsg) => void;
  private readonly onErrorMessage: (message: string) => void;
  private readonly onOutput: (bytes: Uint8Array) => void;
  private readonly onPresence: (presence: PresenceMsg) => void;
  private readonly onStateChange: (state: ConnectionState) => void;
  private readonly url: string;
  private socket: SocketLike | null = null;

  constructor(options: RelayClientOptions) {
    this.url = options.url;
    this.createSocket = options.createSocket ?? ((url) => new WebSocket(url));
    this.onAuthResult = (result) => {
      options.onAuthResult?.(result);
    };
    this.onControlChanged = (control) => {
      options.onControlChanged?.(control);
    };
    this.onPresence = (presence) => {
      options.onPresence?.(presence);
    };
    this.onStateChange = (state) => {
      options.onStateChange(state);
    };
    this.onOutput = (bytes) => {
      options.onOutput(bytes);
    };
    this.onErrorMessage = (message) => {
      options.onErrorMessage(message);
    };
  }

  connect(): void {
    this.onStateChange('connecting');

    const socket = this.createSocket(this.url);
    this.socket = socket;

    socket.onopen = () => {
      this.onStateChange('connected');
    };

    socket.onclose = () => {
      this.onStateChange('disconnected');
    };

    socket.onerror = () => {
      this.onErrorMessage('Connection failed');
    };

    socket.onmessage = (event) => {
      const message = parseRelayMessage(event.data);

      if (!message) {
        return;
      }

      if (message.type === 'output') {
        this.onOutput(decodeBase64Bytes(message.data_base64));
        return;
      }

      if (message.type === 'auth_result') {
        this.onAuthResult(message);
        return;
      }

      if (message.type === 'control_changed') {
        this.onControlChanged(message);
        return;
      }

      if (message.type === 'presence') {
        this.onPresence(message);
        return;
      }

      this.onErrorMessage(message.message);
    };
  }

  sendAuth(message: AuthMessage): void {
    this.socket?.send(JSON.stringify(message));
  }

  sendTakeControl(): void {
    this.socket?.send(JSON.stringify({ v: 1, type: 'take_control' }));
  }

  disconnect(): void {
    this.socket?.close();
    this.socket = null;
  }
}
