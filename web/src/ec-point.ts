import { p256 } from '@noble/curves/nist.js';

/**
 * Decompresses a SEC1-compressed P-256 public key point (33 bytes: a
 * 0x02/0x03 parity prefix + the X coordinate) back to the uncompressed
 * form (65 bytes: 0x04 || X || Y) that WebCrypto's `importKey('raw', ...)`
 * requires.
 *
 * This delegates entirely to `@noble/curves` (an established, audited
 * elliptic-curve library) rather than hand-rolling the modular-square-root
 * math a from-scratch decompression would need - the counterpart of
 * `crypto/elliptic.MarshalCompressed`, which produces this format on the
 * Go side (see internal/hostauth/hostauth.go's
 * PublicKeyCompressedBase64URL). The two are verified to interoperate
 * byte-for-byte, not just independently "look correct."
 */
export function decompressP256PublicKey(compressed: Uint8Array): Uint8Array {
  return p256.Point.fromBytes(compressed).toBytes(false);
}
