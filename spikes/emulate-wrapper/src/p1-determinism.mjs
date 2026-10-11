// P1: print every ID/ts/event id/delivery for a fixed scenario. Usage: node src/p1-determinism.mjs <seed>
import http from 'node:http';
import { createWrapper } from './wrapper.mjs';

const seed = process.argv[2] ?? 'seed-a';
const baseUrl = 'http://127.0.0.1:1';
const w = await createWrapper({ baseUrl, seed });

const received = [];
const recv = http.createServer(async (req, res) => {
  const chunks = []; for await (const c of req) chunks.push(c);
  const body = Buffer.concat(chunks).toString();
  received.push({ sig: req.headers['x-slack-signature'], ts: req.headers['x-slack-request-timestamp'], body: JSON.parse(body) });
  res.end('ok');
}).listen(0, '127.0.0.1');
await new Promise((r) => recv.once('listening', r));
w.inner.webhooks.register({ url: `http://127.0.0.1:${recv.address().port}/e`, events: ['*'], active: true, owner: 'slack' });

const users = w.ss().users.all();
const id = (n) => users.find((u) => u.name === n).user_id;
const call = async (tok, method, params) => {
  const r = await w.fetch(new Request(`${baseUrl}/api/${method}`, { method: 'POST', headers: { authorization: `Bearer ${tok}`, 'content-type': 'application/json; charset=utf-8' }, body: JSON.stringify(params) }));
  return r.json();
};
const alice = `xoxp-${id('alice')}`;
const out = { users: users.map((u) => [u.name, u.user_id, u.created_at]) };
out.channels_seeded = w.ss().channels.all().map((c) => [c.name, c.channel_id, c.created_at]);
const pub = await call(alice, 'conversations.create', { name: 'proj-public' });
const priv = await call(alice, 'conversations.create', { name: 'proj-private', is_private: true });
const dm = await call(alice, 'conversations.open', { users: id('bob') });
const m1 = await call(alice, 'chat.postMessage', { channel: pub.channel.id, text: 'one' });
const m2 = await call(alice, 'chat.postMessage', { channel: pub.channel.id, text: 'two' });
const m3 = await call(alice, 'chat.postMessage', { channel: pub.channel.id, text: 'reply', thread_ts: m1.ts });
const rx = await call(alice, 'reactions.add', { channel: pub.channel.id, timestamp: m2.ts, name: 'thumbsup' });
out.api = { pub: pub.channel?.id ?? pub, priv: priv.channel?.id ?? priv, dm: dm.channel?.id ?? dm, m1: m1.ts ?? m1, m2: m2.ts ?? m2, m3: m3.ts ?? m3, rx };
out.rows = {
  channels: w.ss().channels.all().map((c) => [c.name, c.channel_id, c.created_at, c.updated_at]),
  messages: w.ss().messages.all().map((m) => [m.ts, m.channel_id, m.created_at]),
};
await new Promise((r) => setTimeout(r, 200));
out.received = received.map((r) => ({ sig: r.sig, ts: r.ts, event_id: r.body.event_id, event_time: r.body.event_time, type: r.body.event.type, ev_ts: r.body.event.ts }));
out.deliveries = w.inner.webhooks.getDeliveries().map((d) => [d.id, d.event, d.delivered_at, d.status_code, d.success]);
console.log(JSON.stringify(out, null, 1));
recv.close();
process.exit(0);
