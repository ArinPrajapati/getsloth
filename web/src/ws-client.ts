import type { AuthMessage } from './auth';
import {
  decodeBase64Bytes,
  encodeBase64Bytes,
  parseRelayMessage,
  type AuthResultMsg,
  type ChatBroadcastMsg,
  type ControlChangedMsg,
  type KickedMsg,
  type PresenceMsg,
  type SessionEndedMsg,
  type TerminalSizeMsg
} from './protocol';

export type ConnectionState = 'idle' | 'connecting' | 'connected' | 'disconnected';

// docs/protocol.md Reconnect: a viewer that briefly loses network sends
// resume{token} instead of auth{...} - the relay-issued token is valid
// for RECONNECT_WINDOW_MS after a drop. Backoff is a fixed short delay
// rather than exponential since the window itself is the real bound.
const RECONNECT_WINDOW_MS = 30000;
const RECONNECT_BACKOFF_MS = 1000;

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
  onChatMessage?(message: ChatBroadcastMsg): void;
  onControlChanged?(control: ControlChangedMsg): void;
  onPresence?(presence: PresenceMsg): void;
  onKicked?(message: KickedMsg): void;
  onSessionEnded?(message: SessionEndedMsg): void;
  onTerminalSize?(message: TerminalSizeMsg): void;
}

export class RelayClient {
  private readonly createSocket: (url: string) => SocketLike;
  private readonly onAuthResult: (result: AuthResultMsg) => void;
  private readonly onChatMessage: (message: ChatBroadcastMsg) => void;
  private readonly onControlChanged: (control: ControlChangedMsg) => void;
  private readonly onErrorMessage: (message: string) => void;
  private readonly onKicked: (message: KickedMsg) => void;
  private readonly onOutput: (bytes: Uint8Array) => void;
  private readonly onPresence: (presence: PresenceMsg) => void;
  private readonly onSessionEnded: (message: SessionEndedMsg) => void;
  private readonly onTerminalSize: (message: TerminalSizeMsg) => void;
  private readonly onStateChange: (state: ConnectionState) => void;
  private readonly url: string;
  private socket: SocketLike | null = null;
  private token: string | null = null;
  private intentionalClose = false;
  private reconnectDeadline: number | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(options: RelayClientOptions) {
    this.url = options.url;
    this.createSocket = options.createSocket ?? ((url) => new WebSocket(url));
    this.onAuthResult = (result) => {
      options.onAuthResult?.(result);
    };
    this.onChatMessage = (message) => {
      options.onChatMessage?.(message);
    };
    this.onControlChanged = (control) => {
      options.onControlChanged?.(control);
    };
    this.onPresence = (presence) => {
      options.onPresence?.(presence);
    };
    this.onKicked = (message) => {
      options.onKicked?.(message);
    };
    this.onSessionEnded = (message) => {
      options.onSessionEnded?.(message);
    };
    this.onTerminalSize = (message) => {
      options.onTerminalSize?.(message);
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
    this.intentionalClose = false;
    this.openSocket(false);
  }

  private openSocket(isResume: boolean): void {
    this.onStateChange('connecting');

    const socket = this.createSocket(this.url);
    this.socket = socket;

    socket.onopen = () => {
      if (isResume && this.token) {
        socket.send(JSON.stringify({ v: 1, type: 'resume', token: this.token }));
      }

      this.onStateChange('connected');
    };

    socket.onclose = () => {
      this.onStateChange('disconnected');
      this.scheduleReconnect();
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
        if (message.ok && message.token) {
          this.token = message.token;
          this.reconnectDeadline = null;
        }

        this.onAuthResult(message);
        return;
      }

      if (message.type === 'chat_message') {
        this.onChatMessage(message);
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

      if (message.type === 'kicked') {
        this.onKicked(message);
        return;
      }

      if (message.type === 'session_ended') {
        this.onSessionEnded(message);
        return;
      }

      if (message.type === 'terminal_size') {
        this.onTerminalSize(message);
        return;
      }

      this.onErrorMessage(message.message);
    };
  }

  sendAuth(message: AuthMessage): void {
    this.socket?.send(JSON.stringify(message));
  }

  sendChatMessage(text: string): void {
    this.socket?.send(JSON.stringify({ v: 1, type: 'chat_message', text }));
  }

  sendInput(bytes: Uint8Array): void {
    this.socket?.send(JSON.stringify({ v: 1, type: 'input', data_base64: encodeBase64Bytes(bytes) }));
  }

  sendResize(cols: number, rows: number): void {
    this.socket?.send(JSON.stringify({ v: 1, type: 'resize', cols, rows }));
  }

  sendTakeControl(cols: number, rows: number): void {
    this.socket?.send(JSON.stringify({ v: 1, type: 'take_control', cols, rows }));
  }

  disconnect(): void {
    this.intentionalClose = true;

    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    this.socket?.close();
    this.socket = null;
  }

  private scheduleReconnect(): void {
    if (this.intentionalClose || !this.token) {
      return;
    }

    const now = Date.now();

    if (this.reconnectDeadline === null) {
      this.reconnectDeadline = now + RECONNECT_WINDOW_MS;
    }

    if (now >= this.reconnectDeadline) {
      return;
    }

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.openSocket(true);
    }, RECONNECT_BACKOFF_MS);
  }
}
