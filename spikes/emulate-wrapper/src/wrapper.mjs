// LocalSlack-on-emulate wrapper: fetch-level layer over @emulators/core + @emulators/slack.
import { createHmac, randomBytes } from 'node:crypto';
import { START_MS as DETERMINISTIC_START_MS, advanceClock, installDeterminism, realNow, seedRandom, setClock } from './determinism.mjs';
import { BOT_NAME, seedConfig } from './seed.mjs';

const BOOL_PARAMS = new Set(['is_private', 'exclude_archived', 'include_locale', 'include_num_members', 'reply_broadcast',
  'unfurl_links', 'unfurl_media', 'mrkdwn', 'as_user', 'inclusive', 'include_all_metadata', 'include_disabled', 'link_names']);
const json = (obj, status = 200) => Response.json(obj, { status });
const slackErr = (error, status = 200) => json({ ok: false, error }, status);
const GENERIC_SCOPES = ['chat:write', 'channels:read', 'channels:history', 'groups:read', 'groups:history',
  'im:read', 'im:history', 'im:write', 'users:read', 'reactions:write', 'reactions:read', 'channels:manage'];

export async function createWrapper({ baseUrl, seed = null, unknownMethod = 'go', realClockSignatures = true, modules = null }) {
  const deterministic = seed !== null;
  if (deterministic) installDeterminism({ seed, startMs: DETERMINISTIC_START_MS, stepMs: 0 });
  // `modules` lets a bundler entry (bun --compile) pass statically imported packages; otherwise import dynamically
  // after the determinism patch, with a cache-busting query so reset gets a fresh slack module (resets tsCounter).
  const core = modules?.core ?? (await import('@emulators/core'));
  const slackUrl = modules ? null : import.meta.resolve('@emulators/slack');

  const w = {
    calls: [], seq: 0, gen: 0, retries: [], config: { bad_signature: false, stale_signature: false },
    realClockSignatures,
    sockets: new Set(), pending: new Map(), sinkId: null, inner: null,
  };

  async function build() {
    // Fresh module instance per generation: resets slack's module-private tsCounter.
    globalThis.__resetTsCounter?.(); // only present in the Bun build (patched source)
    const slack = modules?.slack ?? (await import(w.gen === 0 ? slackUrl : `${slackUrl}?gen=${w.gen}`));
    const inner = core.createServer(slack.slackPlugin, { baseUrl, tokens: {} });
    if (slack.slackPlugin.seed) slack.slackPlugin.seed(inner.store, baseUrl);
    slack.seedFromConfig(inner.store, baseUrl, seedConfig);
    w.ss = () => slack.getSlackStore(inner.store);
    const users = w.ss().users;
    const bot = users.all().find((u) => u.name === BOT_NAME);
    users.update(bot.id, { is_bot: true });
    const orig = inner.webhooks.headerFactory;
    inner.webhooks.setHeaderFactory((ctx) => {
      const h = orig(ctx);
      const secret = inner.store.getData('slack.signing_secret');
      if (secret && h['X-Slack-Signature']) {
        // The virtual clock (deterministic mode) is far from wall time; receivers reject timestamps >5 min old,
        // so sign with real time unless asked to stay byte-stable (P1) or to send a stale one on purpose.
        let ts = h['X-Slack-Request-Timestamp'];
        if (w.realClockSignatures) ts = String(Math.floor(realNow() / 1000));
        if (w.config.stale_signature) ts = String(Number(ts) - 600);
        h['X-Slack-Request-Timestamp'] = ts;
        h['X-Slack-Signature'] = `v0=${createHmac('sha256', secret).update(`v0:${ts}:${ctx.body}`).digest('hex')}`;
        if (w.config.bad_signature) h['X-Slack-Signature'] = `v0=${'0'.repeat(64)}`;
      }
      return h;
    });
    w.inner = inner;
    w.origHeaders = orig;
    w.sinkId = null;
    if (w.sockets.size) ensureSink();
  }

  const state = () => w.inner.store;
  const slackData = () => {
    return { users: w.ss().users.all() };
  };

  function resolveToken(token) {
    if (w.inner.tokenMap.has(token)) return true;
    const { users } = slackData();
    let user;
    if (token.startsWith('xoxp-')) user = users.find((u) => u.user_id === token.slice(5));
    else if (token.startsWith('xoxb-')) user = users.find((u) => u.is_bot);
    if (!user) return false;
    w.inner.tokenMap.set(token, { login: user.user_id, id: user.id, scopes: GENERIC_SCOPES });
    return true;
  }

  function authShim(req) {
    const h = req.headers.get('authorization');
    if (!h) return null;
    const token = h.replace(/^(Bearer|token)\s+/i, '').trim();
    if (token.startsWith('xapp-')) return null;
    if (!/^xox[bp]-/.test(token)) return slackErr('invalid_auth');
    if (!resolveToken(token)) return slackErr('invalid_auth');
    return null;
  }

  // ---- Socket Mode -------------------------------------------------------
  function ensureSink() {
    if (w.sinkId !== null) return;
    const sub = w.inner.webhooks.register({ url: `${baseUrl}/_localslack/socket-sink`, events: ['*'], active: true, owner: 'slack' });
    w.sinkId = sub.id;
  }
  function sendEnvelope(payload, attempt = 0, reason, envelopeId) {
    const envelope_id = envelopeId ?? randomBytes(9).toString('base64url');
    const env = { envelope_id, type: 'events_api', payload, accepts_response_payload: false };
    if (attempt) Object.assign(env, { retry_attempt: attempt, retry_reason: reason });
    for (const ws of w.sockets) ws.send(JSON.stringify(env));
    const timer = setTimeout(() => {
      if (!w.pending.has(envelope_id)) return;
      if (attempt >= 2) { w.pending.set(envelope_id, { ...w.pending.get(envelope_id), unacked: true }); return; }
      sendEnvelope(payload, attempt + 1, 'timeout', envelope_id);
    }, w.ackTimeoutMs ?? 3000);
    timer.unref?.();
    w.pending.set(envelope_id, { payload, attempt, acked: false });
  }
  w.attachSocket = (ws) => {
    w.sockets.add(ws);
    ensureSink();
    ws.send(JSON.stringify({ type: 'hello', num_connections: w.sockets.size, connection_info: { app_id: 'A000000001' }, debug_info: { host: 'localslack', approximate_connection_time: 18060 } }));
    ws.on('message', (data) => {
      let m; try { m = JSON.parse(String(data)); } catch { return; }
      if (m.envelope_id && w.pending.has(m.envelope_id)) { w.pending.delete(m.envelope_id); w.acks = (w.acks ?? 0) + 1; w.ackLog = [...(w.ackLog ?? []), m.envelope_id]; }
    });
    ws.on('close', () => w.sockets.delete(ws));
  };

  // ---- _localslack routes --------------------------------------------------
  const deliveriesView = () => {
    const base = w.inner.webhooks.getDeliveries().map((d) => ({
      id: d.id, hook_id: d.hook_id, event_id: d.payload?.event_id, event_type: d.payload?.event?.type,
      status_code: d.status_code, success: d.success, delivered_at: d.delivered_at, duration: d.duration, retry_num: 0,
    }));
    return [...base, ...w.retries];
  };

  async function verifyUrl(url) {
    const challenge = randomBytes(12).toString('hex');
    const body = JSON.stringify({ type: 'url_verification', challenge, token: 'localslack' });
    const headers = w.inner.webhooks.headerFactory({ event: 'url_verification', action: undefined, body, subscription: {}, deliveryId: 0 });
    try {
      const r = await fetch(url, { method: 'POST', headers, body, signal: AbortSignal.timeout(5000) });
      const text = await r.text();
      let echoed = text.trim();
      try { echoed = JSON.parse(text).challenge ?? echoed; } catch { /* plain text */ }
      return r.ok && echoed === challenge;
    } catch { return false; }
  }

  async function internal(req, url) {
    const p = url.pathname;
    const m = req.method;
    if (p === '/_localslack/health') return json({ ok: true });
    if (p === '/_localslack/calls') return json(w.calls);
    if (p === '/_localslack/socket') return json({ connections: w.sockets.size, acks: w.acks ?? 0, ack_log: w.ackLog ?? [], pending: [...w.pending].map(([id, v]) => ({ envelope_id: id, attempt: v.attempt, unacked: !!v.unacked })) });
    if (p === '/_localslack/deliveries') return json(deliveriesView());
    if (p === '/_localslack/config') {
      if (m === 'POST') Object.assign(w.config, await req.json());
      return json(w.config);
    }
    if (p === '/_localslack/reset' && m === 'POST') {
      w.gen += 1; w.calls = []; w.seq = 0; w.retries = []; w.pending.clear();
      if (deterministic) { seedRandom(seed); setClock(DETERMINISTIC_START_MS, 0); }
      await build();
      return json({ ok: true });
    }
    if (p === '/_localslack/clock' && m === 'POST') {
      const { advance_ms } = await req.json(); advanceClock(advance_ms ?? 0);
      return json({ ok: true, now: Date.now() });
    }
    if (p === '/_localslack/snapshot' && m === 'POST') { w.snap = w.inner.store.snapshot(); return json({ ok: true }); }
    if (p === '/_localslack/restore' && m === 'POST') { w.inner.store.restore(w.snap); return json({ ok: true }); }
    if (p === '/_localslack/subscriptions') {
      if (m === 'GET') return json(w.inner.webhooks.getSubscriptions('slack').filter((s) => s.id !== w.sinkId));
      if (m === 'POST') {
        const { url: target, events = ['*'] } = await req.json();
        if (!(await verifyUrl(target))) return json({ ok: false, error: 'url_verification_failed' }, 400);
        const sub = w.inner.webhooks.register({ url: target, events, active: true, owner: 'slack' });
        return json({ ok: true, subscription: sub });
      }
    }
    if (p === '/_localslack/socket-sink' && m === 'POST') {
      sendEnvelope(await req.json());
      return json({ ok: true });
    }
    const retry = p.match(/^\/_localslack\/events\/([^/]+)\/retry$/);
    if (retry && m === 'POST') {
      const body = await req.json().catch(() => ({}));
      const d = w.inner.webhooks.getDeliveries().find((x) => x.payload?.event_id === retry[1]);
      if (!d) return json({ ok: false, error: 'event_not_found' }, 404);
      const sub = w.inner.webhooks.getSubscription(d.hook_id);
      const num = w.retries.filter((r) => r.event_id === retry[1]).length + 1;
      const raw = JSON.stringify(d.payload);
      const headers = w.inner.webhooks.headerFactory({ event: d.event, action: d.action, body: raw, subscription: sub, deliveryId: d.id });
      headers['X-Slack-Retry-Num'] = String(num);
      headers['X-Slack-Retry-Reason'] = body.reason ?? 'http_timeout';
      let status = null;
      try { status = (await fetch(sub.url, { method: 'POST', headers, body: raw, signal: AbortSignal.timeout(10000) })).status; } catch { /* leave null */ }
      const rec = { id: `r${num}-${d.id}`, hook_id: d.hook_id, event_id: retry[1], event_type: d.payload?.event?.type, status_code: status, success: status !== null && status < 300, retry_num: num, retry_reason: headers['X-Slack-Retry-Reason'] };
      w.retries.push(rec);
      return json({ ok: true, ...rec });
    }
    if (p.startsWith('/_localslack/')) return json({ ok: false, error: 'unknown_localslack_route' }, 404);
    return null;
  }

  async function slackSpecific(req, url) {
    if (url.pathname === '/api/apps.connections.open') {
      const t = (req.headers.get('authorization') ?? '').replace(/^Bearer\s+/i, '');
      if (!t.startsWith('xapp-')) return slackErr('not_allowed_token_type');
      return json({ ok: true, url: `${baseUrl.replace(/^http/, 'ws')}/link?ticket=${randomBytes(6).toString('hex')}` });
    }
    return null;
  }

  // State-aware validation emulate lacks. Needs the store, so it cannot live in a pure HTTP proxy.
  async function params(req) {
    const text = await req.clone().text();
    if ((req.headers.get('content-type') ?? '').includes('json')) { try { return JSON.parse(text); } catch { return {}; } }
    return Object.fromEntries(new URLSearchParams(text));
  }
  async function validate(req, url) {
    if (req.method !== 'POST') return null;
    const path = url.pathname;
    if (path === '/api/conversations.list') {
      const p = await params(req);
      const ok = new Set(['public_channel', 'private_channel', 'mpim', 'im']);
      if (p.types && !p.types.split(',').every((t) => ok.has(t))) return slackErr('invalid_types');
      if (p.cursor && !w.ss().channels.findOneBy('channel_id', p.cursor)) return slackErr('invalid_cursor');
    }
    if (path === '/api/chat.postMessage') {
      const p = await params(req);
      if (p.thread_ts && p.channel && !w.ss().messages.all().some((m) => m.ts === p.thread_ts && m.channel_id === p.channel)) {
        if (w.ss().channels.findOneBy('channel_id', p.channel)) return slackErr('thread_not_found');
      }
    }
    return null;
  }

  function shim(res, req, url, seq) {
    const h = new Headers(res.headers);
    for (const k of [...h.keys()]) if (k.startsWith('x-ratelimit-')) h.delete(k);
    if (url.pathname.startsWith('/api/')) {
      h.set('x-slack-req-id', `ls-${seq.toString(16).padStart(8, '0')}`);
      h.set('content-type', 'application/json; charset=utf-8');
      const tok = (req.headers.get('authorization') ?? '').replace(/^(Bearer|token)\s+/i, '').trim();
      const u = w.inner.tokenMap.get(tok);
      if (u) h.set('x-oauth-scopes', u.scopes.join(','));
    }
    return new Response(res.body, { status: res.status, headers: h });
  }

  // Real Slack accepts method args in the query string; slack_sdk's async client sends them that way
  // with an empty body, and GETs (emulate registers POST only for most methods): fold into a urlencoded POST.
  async function foldQueryIntoBody(req, url) {
    if (!url.pathname.startsWith('/api/') || !['GET', 'POST'].includes(req.method)) return req;
    const ct = req.headers.get('content-type') ?? '';
    if (req.method === 'POST' && ct && !ct.startsWith('application/x-www-form-urlencoded')) return req;
    const form = new URLSearchParams(req.method === 'POST' ? await req.text() : '');
    for (const [k, v] of new URLSearchParams(url.search)) form.append(k, v);
    // slack_sdk (Python) serializes booleans as "1"/"0" (or "True"/"False"); emulate's conversations.create only
    // understands "true". Normalize known boolean params only ("1" is also a legitimate limit/ts value).
    for (const [k, v] of [...form]) {
      if (!BOOL_PARAMS.has(k)) continue;
      if (v === '1' || v === 'True') form.set(k, 'true');
      else if (v === '0' || v === 'False') form.set(k, 'false');
    }
    const headers = new Headers(req.headers);
    headers.delete('content-length');
    headers.set('content-type', 'application/x-www-form-urlencoded');
    return new Request(url.origin + url.pathname, { method: 'POST', headers, body: form.toString() });
  }

  w.fetch = async (req0) => {
    const url = new URL(req0.url);
    const req = await foldQueryIntoBody(req0, url);
    const internalRes = await internal(req, url);
    if (internalRes) return internalRes;
    const seq = ++w.seq;
    let body = '';
    if (req.method === 'POST') body = (await req.clone().text()).replace(/(token=)[^&]+/g, '$1***').slice(0, 120);
    let res = (await slackSpecific(req, url)) ?? (url.pathname.startsWith('/api/') ? authShim(req) ?? (await validate(req, url)) : null);
    if (!res) {
      res = await w.inner.app.fetch(req);
      if (url.pathname.startsWith('/api/') && res.status === 404) {
        res = unknownMethod === 'slack' ? slackErr('unknown_method') : slackErr('not_implemented', 404);
      }
    }
    res = shim(res, req, url, seq);
    w.calls.push({ seq, method: req.method, path: url.pathname, status: res.status, body });
    return res;
  };

  await build();
  return w;
}
