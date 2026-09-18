export type RelayMessage =
  | AuthResultMsg
  | ChatBroadcastMsg
  | ControlChangedMsg
  | ErrorMsg
  | KickedMsg
  | OutputMsg
  | PresenceMsg
  | SessionEndedMsg
  | TerminalSizeMsg;

export type SessionMode = 'remote' | 'group';

export interface TerminalSizeMsg {
  v: 1;
  type: 'terminal_size';
  cols: number;
  rows: number;
}

export interface KickedMsg {
  v: 1;
  type: 'kicked';
  reason: 'kill_switch';
}

export interface SessionEndedMsg {
  v: 1;
  type: 'session_ended';
  reason: 'process_exited' | 'host_ended' | 'host_disconnected';
}

export interface ChatBroadcastMsg {
  v: 1;
  type: 'chat_message';
  sender_id: string;
  sender_role: 'host' | 'viewer';
  sender_display_name?: string;
  text: string;
}

export interface ControlChangedMsg {
  v: 1;
  type: 'control_changed';
  active_writer_id: string;
  active_writer_role: 'host' | 'viewer';
  cols: number;
  rows: number;
}

export interface PresenceConnection {
  id: string;
  role: 'host' | 'viewer';
  display_name?: string;
  is_active_writer: boolean;
}

export interface PresenceMsg {
  v: 1;
  type: 'presence';
  connections: PresenceConnection[];
}

export interface AuthResultMsg {
  v: 1;
  type: 'auth_result';
  ok: boolean;
  token?: string;
  connection_id?: string;
  code?: 'AUTH_FAILED' | 'RATE_LIMITED' | 'SESSION_OCCUPIED';
  retry_after_ms?: number;
  mode?: SessionMode;
  cols?: number;
  rows?: number;
  active_writer_id?: string;
  active_writer_role?: 'host' | 'viewer';
}

export interface ErrorMsg {
  v: 1;
  type: 'error';
  code: 'SESSION_NOT_FOUND' | 'UNAUTHORIZED' | 'NOT_ACTIVE_WRITER' | 'READ_ONLY_SESSION' | 'UNSUPPORTED_VERSION' | 'BAD_REQUEST';
  message: string;
}

export interface OutputMsg {
  v: 1;
  type: 'output';
  data_base64: string;
}

export function parseRelayMessage(raw: string): RelayMessage | null {
  let parsed: unknown;

  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }

  if (!isRecord(parsed) || parsed.v !== 1 || typeof parsed.type !== 'string') {
    return null;
  }

  if (parsed.type === 'output' && typeof parsed.data_base64 === 'string') {
    return { v: 1, type: 'output', data_base64: parsed.data_base64 };
  }

  if (parsed.type === 'error' && isErrorCode(parsed.code) && typeof parsed.message === 'string') {
    return { v: 1, type: 'error', code: parsed.code, message: parsed.message };
  }

  if (parsed.type === 'auth_result' && typeof parsed.ok === 'boolean') {
    return {
      v: 1,
      type: 'auth_result',
      ok: parsed.ok,
      ...(typeof parsed.token === 'string' ? { token: parsed.token } : {}),
      ...(typeof parsed.connection_id === 'string' ? { connection_id: parsed.connection_id } : {}),
      ...(isAuthFailureCode(parsed.code) ? { code: parsed.code } : {}),
      ...(typeof parsed.retry_after_ms === 'number' ? { retry_after_ms: parsed.retry_after_ms } : {}),
      ...(isSessionMode(parsed.mode) ? { mode: parsed.mode } : {}),
      ...(isTerminalDimension(parsed.cols) ? { cols: parsed.cols } : {}),
      ...(isTerminalDimension(parsed.rows) ? { rows: parsed.rows } : {}),
      ...(typeof parsed.active_writer_id === 'string' ? { active_writer_id: parsed.active_writer_id } : {}),
      ...(isRole(parsed.active_writer_role) ? { active_writer_role: parsed.active_writer_role } : {})
    };
  }

  if (
    parsed.type === 'chat_message' &&
    typeof parsed.sender_id === 'string' &&
    isRole(parsed.sender_role) &&
    (typeof parsed.sender_display_name === 'string' || parsed.sender_display_name === undefined) &&
    typeof parsed.text === 'string'
  ) {
    return {
      v: 1,
      type: 'chat_message',
      sender_id: parsed.sender_id,
      sender_role: parsed.sender_role,
      ...(typeof parsed.sender_display_name === 'string' ? { sender_display_name: parsed.sender_display_name } : {}),
      text: parsed.text
    };
  }

  if (
    parsed.type === 'control_changed' &&
    typeof parsed.active_writer_id === 'string' &&
    isRole(parsed.active_writer_role) &&
    isTerminalDimension(parsed.cols) &&
    isTerminalDimension(parsed.rows)
  ) {
    return {
      v: 1,
      type: 'control_changed',
      active_writer_id: parsed.active_writer_id,
      active_writer_role: parsed.active_writer_role,
      cols: parsed.cols,
      rows: parsed.rows
    };
  }

  if (parsed.type === 'terminal_size' && isTerminalDimension(parsed.cols) && isTerminalDimension(parsed.rows)) {
    return { v: 1, type: 'terminal_size', cols: parsed.cols, rows: parsed.rows };
  }

  if (parsed.type === 'presence' && Array.isArray(parsed.connections)) {
    const connections = parsed.connections.filter(isPresenceConnection);

    if (connections.length === parsed.connections.length) {
      return { v: 1, type: 'presence', connections };
    }
  }

  if (parsed.type === 'kicked' && parsed.reason === 'kill_switch') {
    return { v: 1, type: 'kicked', reason: 'kill_switch' };
  }

  if (parsed.type === 'session_ended' && isSessionEndedReason(parsed.reason)) {
    return { v: 1, type: 'session_ended', reason: parsed.reason };
  }

  return null;
}

export function encodeBase64Bytes(bytes: Uint8Array): string {
  let binary = '';

  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }

  return btoa(binary);
}

export function decodeBase64Bytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);

  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }

  return bytes;
}

// docs/protocol.md Limits: input.data_base64, decoded, must be <= 4096
// bytes — violation closes the WHOLE connection (BAD_REQUEST, close code
// 4002), not just the one message, so every input path (quick actions,
// typed keystrokes) has to enforce this client-side before sending.
export const MAX_INPUT_BYTES = 4096;

const textEncoder = new TextEncoder();

export function truncateToByteLimit(text: string, maxBytes: number): Uint8Array {
  const bytes = textEncoder.encode(text);

  if (bytes.length <= maxBytes) {
    return bytes;
  }

  // Back off byte-by-byte until the prefix is valid UTF-8 again, so the cut
  // never lands inside a multi-byte character.
  let end = maxBytes;

  while (end > 0) {
    try {
      new TextDecoder('utf-8', { fatal: true }).decode(bytes.slice(0, end));
      return bytes.slice(0, end);
    } catch {
      end -= 1;
    }
  }

  return new Uint8Array(0);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isErrorCode(value: unknown): value is ErrorMsg['code'] {
  return (
    value === 'SESSION_NOT_FOUND' ||
    value === 'UNAUTHORIZED' ||
    value === 'NOT_ACTIVE_WRITER' ||
    value === 'READ_ONLY_SESSION' ||
    value === 'UNSUPPORTED_VERSION' ||
    value === 'BAD_REQUEST'
  );
}

function isAuthFailureCode(value: unknown): value is NonNullable<AuthResultMsg['code']> {
  return value === 'AUTH_FAILED' || value === 'RATE_LIMITED' || value === 'SESSION_OCCUPIED';
}

function isSessionMode(value: unknown): value is SessionMode {
  return value === 'remote' || value === 'group';
}

function isTerminalDimension(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

function isPresenceConnection(value: unknown): value is PresenceConnection {
  return (
    isRecord(value) &&
    typeof value.id === 'string' &&
    isRole(value.role) &&
    (typeof value.display_name === 'string' || value.display_name === undefined) &&
    typeof value.is_active_writer === 'boolean'
  );
}

function isRole(value: unknown): value is 'host' | 'viewer' {
  return value === 'host' || value === 'viewer';
}

function isSessionEndedReason(value: unknown): value is SessionEndedMsg['reason'] {
  return value === 'process_exited' || value === 'host_ended' || value === 'host_disconnected';
}
