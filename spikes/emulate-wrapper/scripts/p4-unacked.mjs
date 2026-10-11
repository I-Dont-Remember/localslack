// P4 extra: an envelope that is never acked is resent (retry_attempt 1,2 with retry_reason) then marked unacked.
import WebSocket from 'ws';
const URL_ = process.env.LOCALSLACK_URL;
const open = await (await fetch(`${URL_}/api/apps.connections.open`, { method: 'POST', headers: { authorization: 'Bearer xapp-1-t' } })).json();
const ws = new WebSocket(open.url);
const frames = [];
ws.on('message', (d) => frames.push(JSON.parse(String(d))));
await new Promise((r) => ws.once('open', r));
const users = await (await fetch(`${URL_}/api/users.list`, { method: 'POST', headers: { authorization: 'Bearer xoxb-t' } })).json();
const alice = users.members.find((u) => u.name === 'alice').id;
await fetch(`${URL_}/api/chat.postMessage`, { method: 'POST', headers: { authorization: `Bearer xoxp-${alice}`, 'content-type': 'application/json' }, body: JSON.stringify({ channel: 'C000000001', text: 'never acked' }) });
await new Promise((r) => setTimeout(r, 7500));
console.log(frames.map((f) => ({ type: f.type, envelope_id: f.envelope_id, attempt: f.retry_attempt, reason: f.retry_reason, ev: f.payload?.event?.type })));
console.log('state', JSON.stringify(await (await fetch(`${URL_}/_localslack/socket`)).json()));
process.exit(0);
