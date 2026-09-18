export type MobileTerminalKey =
  | 'escape'
  | 'tab'
  | 'arrow-left'
  | 'arrow-up'
  | 'arrow-down'
  | 'arrow-right';

const mobileTerminalKeySequences: Record<MobileTerminalKey, string> = {
  escape: '\u001b',
  tab: '\t',
  'arrow-left': '\u001b[D',
  'arrow-up': '\u001b[A',
  'arrow-down': '\u001b[B',
  'arrow-right': '\u001b[C'
};

export function mobileTerminalKeyBytes(key: MobileTerminalKey): Uint8Array {
  return new TextEncoder().encode(mobileTerminalKeySequences[key]);
}

export function controlModifiedInput(data: string): string {
  const first = data.codePointAt(0);
  if (first === undefined) {
    return data;
  }

  const upper = String.fromCodePoint(first).toUpperCase();
  if (upper.length !== 1 || upper < 'A' || upper > 'Z') {
    return data;
  }

  return String.fromCharCode(upper.charCodeAt(0) - 64) + data.slice(upper.length);
}
