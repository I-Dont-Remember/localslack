// Seeded PRNG + controllable clock, installed BEFORE @emulators/* load.
// Patches: crypto.randomBytes / randomUUID, Math.random, Date (global), and re-syncs ESM builtin exports.
import crypto from 'node:crypto';
import module from 'node:module';

const RealDate = Date;
export const realNow = () => RealDate.now();
const realRandomBytes = crypto.randomBytes;

function hashSeed(seed) {
  return crypto.createHash('sha256').update(String(seed)).digest();
}

// Counter-mode sha256 stream: simple, portable, and independent of Math.random.
function makeStream(seed) {
  const key = hashSeed(seed);
  let counter = 0;
  let buf = Buffer.alloc(0);
  return (n) => {
    while (buf.length < n) {
      const h = crypto.createHash('sha256').update(key).update(String(counter++)).digest();
      buf = Buffer.concat([buf, h]);
    }
    const out = buf.subarray(0, n);
    buf = buf.subarray(n);
    return Buffer.from(out);
  };
}

const state = { stream: null, nowMs: null, stepMs: 0 };

export function seedRandom(seed) {
  state.stream = makeStream(seed);
}

/** Freeze (stepMs=0) or tick (stepMs>0 per Date.now() call) a virtual clock. null startMs = real time. */
export function setClock(startMs, stepMs = 0) {
  state.nowMs = startMs;
  state.stepMs = stepMs;
}
export function advanceClock(ms) {
  if (state.nowMs !== null) state.nowMs += ms;
}
function clockNow() {
  if (state.nowMs === null) return RealDate.now();
  const v = state.nowMs;
  state.nowMs += state.stepMs;
  return v;
}

class FakeDate extends RealDate {
  constructor(...args) {
    if (args.length === 0) super(clockNow());
    else super(...args);
  }
  static now() {
    return clockNow();
  }
}

function uuidFrom(b) {
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = b.toString('hex');
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

export const START_MS = Date.UTC(2026, 0, 1, 12, 0, 0);
let installed = false;
/** For bundlers whose builtin named imports ignore patches (Bun): crypto-shim.mjs calls these. */
export const isSeeded = () => state.stream !== null;
export const seededRandomBytes = (n) => state.stream(n);
export const seededUUID = () => uuidFrom(state.stream(16));

export function installDeterminism({ seed, startMs = null, stepMs = 0 }) {
  if (installed) { seedRandom(seed); setClock(startMs, stepMs); return; }
  installed = true;
  seedRandom(seed);
  setClock(startMs, stepMs);
  crypto.randomBytes = (n, cb) => {
    const b = state.stream(n);
    if (cb) return void process.nextTick(cb, null, b);
    return b;
  };
  crypto.randomUUID = () => uuidFrom(state.stream(16));
  Math.random = () => state.stream(4).readUInt32BE(0) / 2 ** 32;
  globalThis.Date = FakeDate;
  // Named ESM imports (`import { randomBytes } from "crypto"`) bind to the builtin's export cell.
  module.syncBuiltinESMExports();
}

export { realRandomBytes };
