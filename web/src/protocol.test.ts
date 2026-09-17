import { decodeBase64Bytes, parseRelayMessage } from './protocol';

describe('parseRelayMessage', () => {
  it('accepts relay output messages from protocol v1', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'output', data_base64: 'aGVsbG8=' }));

    expect(message).toEqual({ v: 1, type: 'output', data_base64: 'aGVsbG8=' });
  });

  it('ignores unknown relay message types for forward compatibility', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'future_broadcast' }));

    expect(message).toBeNull();
  });

  it('rejects messages from an unsupported protocol version', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 2, type: 'output', data_base64: 'aGVsbG8=' }));

    expect(message).toBeNull();
  });

  it('rejects malformed JSON and wrong-shaped known messages', () => {
    expect(parseRelayMessage('{')).toBeNull();
    expect(parseRelayMessage(JSON.stringify(null))).toBeNull();
    expect(parseRelayMessage(JSON.stringify({ v: 1, type: 'output' }))).toBeNull();
    expect(parseRelayMessage(JSON.stringify({ v: 1, type: 'error', code: 'NOPE', message: 'bad' }))).toBeNull();
  });
});

describe('decodeBase64Bytes', () => {
  it('decodes base64 terminal bytes without assuming UTF-8 at the protocol boundary', () => {
    const bytes = decodeBase64Bytes('SGkNCg==');

    expect([...bytes]).toEqual([72, 105, 13, 10]);
  });
});
