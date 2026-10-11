// Entry for `bun build --compile`: dynamic import() of a resolved path cannot be bundled, so import statically.
// The crypto module is swapped for src/crypto-shim.mjs at build time; determinism-boot patches Date/Math.random first.
import './determinism-boot.mjs';
import * as core from '@emulators/core';
import * as slack from '@emulators/slack';

globalThis.__LOCALSLACK_MODULES = { core, slack };
await import('./server.mjs');
