import { encodeBase64Bytes } from './protocol';

export interface AuthMessage {
  v: 1;
  type: 'auth';
  viewer_pubkey_base64: string;
  ciphertext_base64: string;
  display_name?: string;
}

export interface CreateAuthMessageOptions {
  sessionId: string;
  hostPublicKeyBase64Url: string;
  password: string;
  displayName?: string;
}

const textEncoder = new TextEncoder();

export class AuthCryptoUnavailableError extends Error {
  constructor() {
    super('Secure browser context required for password encryption. Use HTTPS, localhost, or a trusted local development setup.');
    this.name = 'AuthCryptoUnavailableError';
  }
}

export async function createAuthMessage(options: CreateAuthMessageOptions): Promise<AuthMessage> {
  const browserCrypto = globalThis.crypto as Crypto | undefined;
  const secureContext = (globalThis as { isSecureContext?: boolean }).isSecureContext;

  if (secureContext === false || !browserCrypto?.subtle) {
    throw new AuthCryptoUnavailableError();
  }

  const viewerKeys = await browserCrypto.subtle.generateKey(
    { name: 'ECDH', namedCurve: 'P-256' },
    true,
    ['deriveBits']
  );
  const hostPublicKey = await browserCrypto.subtle.importKey(
    'raw',
    toArrayBuffer(base64UrlToBytes(options.hostPublicKeyBase64Url)),
    { name: 'ECDH', namedCurve: 'P-256' },
    false,
    []
  );
  const sharedSecret = await browserCrypto.subtle.deriveBits(
    { name: 'ECDH', public: hostPublicKey },
    viewerKeys.privateKey,
    256
  );
  const hkdfKey = await browserCrypto.subtle.importKey('raw', sharedSecret, 'HKDF', false, ['deriveKey']);
  const aesKey = await browserCrypto.subtle.deriveKey(
    {
      name: 'HKDF',
      hash: 'SHA-256',
      salt: new ArrayBuffer(0),
      info: toArrayBuffer(textEncoder.encode('getsloth-v1-auth'))
    },
    hkdfKey,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt']
  );
  const nonce = browserCrypto.getRandomValues(new Uint8Array(12));
  const ciphertext = new Uint8Array(
    await browserCrypto.subtle.encrypt(
      {
        name: 'AES-GCM',
        iv: toArrayBuffer(nonce),
        additionalData: toArrayBuffer(textEncoder.encode(`getsloth-auth-v1:${options.sessionId}`)),
        tagLength: 128
      },
      aesKey,
      toArrayBuffer(textEncoder.encode(options.password))
    )
  );
  const viewerPublicKey = new Uint8Array(await browserCrypto.subtle.exportKey('raw', viewerKeys.publicKey));
  const message: AuthMessage = {
    v: 1,
    type: 'auth',
    viewer_pubkey_base64: encodeBase64Bytes(viewerPublicKey),
    ciphertext_base64: encodeBase64Bytes(concatBytes(nonce, ciphertext))
  };

  if (options.displayName) {
    message.display_name = options.displayName;
  }

  return message;
}

export function rawPublicKeyToFragmentKey(bytes: Uint8Array): string {
  return encodeBase64Bytes(bytes).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

function base64UrlToBytes(value: string): Uint8Array {
  const padded = value.replaceAll('-', '+').replaceAll('_', '/').padEnd(Math.ceil(value.length / 4) * 4, '=');
  return Uint8Array.from(atob(padded), (char) => char.charCodeAt(0));
}

function concatBytes(first: Uint8Array, second: Uint8Array): Uint8Array {
  const combined = new Uint8Array(first.length + second.length);
  combined.set(first);
  combined.set(second, first.length);
  return combined;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}
