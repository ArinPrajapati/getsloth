import { decompressP256PublicKey } from './ec-point';

function base64UrlToBytes(value: string): Uint8Array {
  const padded = value.replaceAll('-', '+').replaceAll('_', '/').padEnd(Math.ceil(value.length / 4) * 4, '=');
  return Uint8Array.from(atob(padded), (char) => char.charCodeAt(0));
}

function bytesToHex(bytes: Uint8Array): string {
  return [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
}

describe('decompressP256PublicKey', () => {
  // Generated independently by cmd/getsloth's internal/hostauth package
  // (Go's crypto/ecdh for the uncompressed form, crypto/elliptic's
  // MarshalCompressed for the compressed form) - a real cross-language
  // vector, not values derived from this test's own code path.
  const uncompressedBase64Url = 'BMaHXe3uq0Nnnr4F3pg2dsDXjAeOyG9bGOQks8OsIgC1nsc020-2UYarwhAnZtEgtKtHZgys5agbXoISsfrf5f4';
  const compressedBase64Url = 'AsaHXe3uq0Nnnr4F3pg2dsDXjAeOyG9bGOQks8OsIgC1';

  it('decompresses a Go-generated compressed key to match Go\'s own uncompressed bytes', () => {
    const compressed = base64UrlToBytes(compressedBase64Url);
    const uncompressed = base64UrlToBytes(uncompressedBase64Url);

    const decompressed = decompressP256PublicKey(compressed);

    expect(decompressed.length).toBe(65);
    expect(decompressed[0]).toBe(0x04);
    expect(bytesToHex(decompressed)).toBe(bytesToHex(uncompressed));
  });

  it('is importable by WebCrypto as a raw P-256 ECDH key', async () => {
    const compressed = base64UrlToBytes(compressedBase64Url);
    const decompressed = decompressP256PublicKey(compressed);

    const key = await crypto.subtle.importKey(
      'raw',
      decompressed.buffer.slice(decompressed.byteOffset, decompressed.byteOffset + decompressed.byteLength) as ArrayBuffer,
      { name: 'ECDH', namedCurve: 'P-256' },
      false,
      []
    );

    expect(key.type).toBe('public');
  });
});
