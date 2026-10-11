// Single-file executable via Bun. Run: bun scripts/build-bun.mjs
//  - fonts: both packages readFileSync(join(__dirname,"fonts",..)) at import time => embed at <bunfs root>/fonts
//  - crypto: rewrite `from "crypto"` in @emulators/* to src/crypto-shim.mjs (seedable IDs)
import { cpSync, mkdirSync, rmSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(import.meta.dir, '..');
const stage = `${root}/out/stage`;
rmSync(stage, { recursive: true, force: true });
mkdirSync(`${stage}/fonts`, { recursive: true });
cpSync(`${root}/src`, stage, { recursive: true });
cpSync(`${root}/node_modules/@emulators/core/dist/fonts`, `${stage}/fonts`, { recursive: true });

const shim = {
  name: 'crypto-shim',
  // Builtins bypass onResolve, so rewrite the import specifier in the packages' source instead.
  setup(build) {
    build.onLoad({ filter: /@emulators[\\/](core|slack)[\\/]dist[\\/]index\.js$/ }, async (args) => {
      const src = await Bun.file(args.path).text();
      const out = src.replaceAll('from "crypto"', `from "${stage}/crypto-shim.mjs"`)
        // slack's ts counter is module-private; expose a reset hook so POST /_localslack/reset restarts ts at .000001
        .replace('var tsCounter = 0;', 'var tsCounter = 0; globalThis.__resetTsCounter = () => { tsCounter = 0; };');
      return { contents: out, loader: 'js' };
    });
  },
};
const fonts = readdirSync(`${stage}/fonts`).map((f) => `${stage}/fonts/${f}`);
const r = await Bun.build({
  entrypoints: [`${stage}/server-bun.mjs`, ...fonts],
  root: stage,
  plugins: [shim],
  naming: { asset: '[dir]/[name].[ext]' },
  compile: { outfile: `${root}/out/localslack-bun` },
});
if (!r.success) { console.error(r.logs.join('\n')); process.exit(1); }
console.log('built out/localslack-bun');
