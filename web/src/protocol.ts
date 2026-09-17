export type RelayMessage =
  | AuthResultMsg
  | ChatBroadcastMsg
  | ControlChangedMsg
  | ErrorMsg
  | KickedMsg
  | OutputMsg
  | PresenceMsg
  | SessionEndedMsg;

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
  code?: 'AUTH_FAILED' | 'RATE_LIMITED';
  retry_after_ms?: number;
}

export interface ErrorMsg {
  v: 1;
  type: 'error';
  code: 'SESSION_NOT_FOUND' | 'UNAUTHORIZED' | 'NOT_ACTIVE_WRITER' | 'UNSUPPORTED_VERSION' | 'BAD_REQUEST';
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
      ...(typeof parsed.retry_after_ms === 'number' ? { retry_after_ms: parsed.retry_after_ms } : {})
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
    isRole(parsed.active_writer_role)
  ) {
    return {
      v: 1,
      type: 'control_changed',
      active_writer_id: parsed.active_writer_id,
      active_writer_role: parsed.active_writer_role
    };
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function isErrorCode(value: unknown): value is ErrorMsg['code'] {
  return (
    value === 'SESSION_NOT_FOUND' ||
    value === 'UNAUTHORIZED' ||
    value === 'NOT_ACTIVE_WRITER' ||
    value === 'UNSUPPORTED_VERSION' ||
    value === 'BAD_REQUEST'
  );
}

function isAuthFailureCode(value: unknown): value is NonNullable<AuthResultMsg['code']> {
  return value === 'AUTH_FAILED' || value === 'RATE_LIMITED';
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
