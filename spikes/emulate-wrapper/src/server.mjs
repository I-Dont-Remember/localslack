// Entry: node src/server.mjs   (env: PORT, HOST, LOCALSLACK_SEED, UNKNOWN_METHOD=go|slack)
import http from 'node:http';
import { WebSocketServer } from 'ws';
import { createWrapper } from './wrapper.mjs';

const host = process.env.HOST ?? '127.0.0.1';
const port = Number(process.env.PORT ?? 3232);
const baseUrl = `http://${host}:${port}`;
const w = await createWrapper({
  baseUrl,
  seed: process.env.LOCALSLACK_SEED || null,
  unknownMethod: process.env.UNKNOWN_METHOD ?? 'go',
});

const server = http.createServer(async (nreq, nres) => {
  const chunks = [];
  for await (const c of nreq) chunks.push(c);
  const body = chunks.length ? Buffer.concat(chunks) : undefined;
  const headers = new Headers();
  for (const [k, v] of Object.entries(nreq.headers)) if (v !== undefined) headers.set(k, Array.isArray(v) ? v.join(', ') : v);
  const hasBody = !['GET', 'HEAD'].includes(nreq.method);
  const res = await w.fetch(new Request(baseUrl + nreq.url, { method: nreq.method, headers, body: hasBody ? body : undefined }));
  nres.writeHead(res.status, Object.fromEntries(res.headers));
  nres.end(Buffer.from(await res.arrayBuffer()));
});

const wss = new WebSocketServer({ noServer: true });
server.on('upgrade', (req, socket, head) => {
  if (!req.url.startsWith('/link')) return socket.destroy();
  wss.handleUpgrade(req, socket, head, (ws) => w.attachSocket(ws));
});

server.listen(port, host, () => console.log(`listening ${baseUrl}`));
for (const sig of ['SIGINT', 'SIGTERM']) process.on(sig, () => process.exit(0));
