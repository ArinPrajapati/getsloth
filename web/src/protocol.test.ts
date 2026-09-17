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

  it('accepts kicked messages', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'kicked', reason: 'kill_switch' }));

    expect(message).toEqual({ v: 1, type: 'kicked', reason: 'kill_switch' });
  });

  it('rejects a kicked message with an unknown reason', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'kicked', reason: 'something_else' }));

    expect(message).toBeNull();
  });

  it('accepts session_ended messages for every known reason', () => {
    for (const reason of ['process_exited', 'host_ended', 'host_disconnected']) {
      const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'session_ended', reason }));

      expect(message).toEqual({ v: 1, type: 'session_ended', reason });
    }
  });

  it('rejects a session_ended message with an unknown reason', () => {
    const message = parseRelayMessage(JSON.stringify({ v: 1, type: 'session_ended', reason: 'nope' }));

    expect(message).toBeNull();
  });
});

describe('decodeBase64Bytes', () => {
  it('decodes base64 terminal bytes without assuming UTF-8 at the protocol boundary', () => {
    const bytes = decodeBase64Bytes('SGkNCg==');

    expect([...bytes]).toEqual([72, 105, 13, 10]);
  });
});
