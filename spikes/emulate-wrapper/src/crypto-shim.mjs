// Stand-in for `crypto` when bundling the emulate packages with Bun (see scripts/build-bun.mjs).
// Bun's named builtin imports do not observe patches to the crypto default export, so route the two
// nondeterministic functions through the seeded stream and pass everything else through.
import * as real from 'node:crypto';
import { isSeeded, seededRandomBytes, seededUUID } from './determinism.mjs';

export const { createHmac, createPublicKey, timingSafeEqual, createHash } = real;
export const randomBytes = (n, cb) => (isSeeded() && !cb ? seededRandomBytes(n) : real.randomBytes(n, cb));
export const randomUUID = () => (isSeeded() ? seededUUID() : real.randomUUID());
export default { ...real, randomBytes, randomUUID };
