import { p256 } from '@noble/curves/nist.js';
import { AuthCryptoUnavailableError, createAuthMessage, rawPublicKeyToFragmentKey } from './auth';

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();

describe('createAuthMessage', () => {
  it('explains when browser crypto is unavailable', async () => {
    const original = Object.getOwnPropertyDescriptor(globalThis, 'isSecureContext');
    Object.defineProperty(globalThis, 'isSecureContext', { configurable: true, value: false });

    try {
      await expect(createAuthMessage({
        sessionId: 'session-123',
        hostPublicKeyBase64Url: 'bad-key',
        password: 'secret'
      })).rejects.toThrow(AuthCryptoUnavailableError);
    } finally {
      if (original) {
        Object.defineProperty(globalThis, 'isSecureContext', original);
      } else {
        Reflect.deleteProperty(globalThis, 'isSecureContext');
      }
    }
  });

  it('encrypts the password to the host public key using the protocol wire format', async () => {
    const hostKeys = await crypto.subtle.generateKey(
      { name: 'ECDH', namedCurve: 'P-256' },
      true,
      ['deriveBits']
    );
    const hostPublicRaw = new Uint8Array(await crypto.subtle.exportKey('raw', hostKeys.publicKey));
    const hostFragmentKey = rawPublicKeyToFragmentKey(hostPublicRaw);

    const message = await createAuthMessage({
      sessionId: 'session-123',
      hostPublicKeyBase64Url: hostFragmentKey,
      password: 'correct horse battery staple',
      displayName: 'Phone'
    });

    const viewerPublicKey = await crypto.subtle.importKey(
      'raw',
      Uint8Array.from(atob(message.viewer_pubkey_base64), (char) => char.charCodeAt(0)),
      { name: 'ECDH', namedCurve: 'P-256' },
      false,
      []
    );
    const sharedSecret = await crypto.subtle.deriveBits(
      { name: 'ECDH', public: viewerPublicKey },
      hostKeys.privateKey,
      256
    );
    const hkdfKey = await crypto.subtle.importKey('raw', sharedSecret, 'HKDF', false, ['deriveKey']);
    const aesKey = await crypto.subtle.deriveKey(
      {
        name: 'HKDF',
        hash: 'SHA-256',
        salt: new Uint8Array(),
        info: textEncoder.encode('getsloth-v1-auth')
      },
      hkdfKey,
      { name: 'AES-GCM', length: 256 },
      false,
      ['decrypt']
    );
    const encrypted = Uint8Array.from(atob(message.ciphertext_base64), (char) => char.charCodeAt(0));
    const nonce = encrypted.slice(0, 12);
    const ciphertext = encrypted.slice(12);
    const plaintext = await crypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv: nonce,
        additionalData: textEncoder.encode('getsloth-auth-v1:session-123'),
        tagLength: 128
      },
      aesKey,
      ciphertext
    );

    expect(message).toMatchObject({ v: 1, type: 'auth', display_name: 'Phone' });
    expect(textDecoder.decode(plaintext)).toBe('correct horse battery staple');
  });

  it('accepts a compressed host public key (the QR-only link format) and still derives the same shared secret', async () => {
    // Mirrors the previous test, but the fragment key is the QR link's
    // compressed form (see cmd/getsloth/shareurl.go's qrShareURL) instead
    // of the plain link's uncompressed one - createAuthMessage must
    // decompress it internally before this test's manual host-side ECDH
    // (which, like the real host in internal/hostauth, only ever handles
    // the uncompressed form) can succeed.
    const hostKeys = await crypto.subtle.generateKey(
      { name: 'ECDH', namedCurve: 'P-256' },
      true,
      ['deriveBits']
    );
    const hostPublicRaw = new Uint8Array(await crypto.subtle.exportKey('raw', hostKeys.publicKey));
    const hostPublicCompressed = p256.Point.fromBytes(hostPublicRaw).toBytes(true);
    const hostFragmentKey = rawPublicKeyToFragmentKey(hostPublicCompressed);

    const message = await createAuthMessage({
      sessionId: 'session-123',
      hostPublicKeyBase64Url: hostFragmentKey,
      password: 'correct horse battery staple'
    });

    const viewerPublicKey = await crypto.subtle.importKey(
      'raw',
      Uint8Array.from(atob(message.viewer_pubkey_base64), (char) => char.charCodeAt(0)),
      { name: 'ECDH', namedCurve: 'P-256' },
      false,
      []
    );
    const sharedSecret = await crypto.subtle.deriveBits(
      { name: 'ECDH', public: viewerPublicKey },
      hostKeys.privateKey,
      256
    );
    const hkdfKey = await crypto.subtle.importKey('raw', sharedSecret, 'HKDF', false, ['deriveKey']);
    const aesKey = await crypto.subtle.deriveKey(
      {
        name: 'HKDF',
        hash: 'SHA-256',
        salt: new Uint8Array(),
        info: textEncoder.encode('getsloth-v1-auth')
      },
      hkdfKey,
      { name: 'AES-GCM', length: 256 },
      false,
      ['decrypt']
    );
    const encrypted = Uint8Array.from(atob(message.ciphertext_base64), (char) => char.charCodeAt(0));
    const plaintext = await crypto.subtle.decrypt(
      {
        name: 'AES-GCM',
        iv: encrypted.slice(0, 12),
        additionalData: textEncoder.encode('getsloth-auth-v1:session-123'),
        tagLength: 128
      },
      aesKey,
      encrypted.slice(12)
    );

    expect(textDecoder.decode(plaintext)).toBe('correct horse battery staple');
  });
});
