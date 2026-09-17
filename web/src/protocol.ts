export type RelayMessage = AuthResultMsg | ErrorMsg | OutputMsg;

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

  return null;
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
