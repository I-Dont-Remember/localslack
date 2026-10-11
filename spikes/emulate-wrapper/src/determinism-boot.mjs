// Import FIRST in bundled entries: static imports evaluate in order, so this patches crypto/Date before the
// emulate packages bind `randomBytes`/`randomUUID` (Bun does not honor syncBuiltinESMExports after the fact).
import { START_MS, installDeterminism } from './determinism.mjs';

if (process.env.LOCALSLACK_SEED) installDeterminism({ seed: process.env.LOCALSLACK_SEED, startMs: START_MS });
