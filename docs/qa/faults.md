# QA: Fault injection

Status: implemented (stream D). Deterministic rate limits, server errors, Slack errors, delays and nth-call faults for `/api/*`, driven by `/_localslack/faults`. Contract: `docs/TESTING_API.md` (Faults).

## Setup

```bash
make run   # LocalSlack on :3232
export URL=http://localhost:3232
export TOKEN=$(printf 'xoxb-%s' qa)   # any xoxb- token resolves to the default bot
call() { curl -s -i -X POST "$URL/api/$1" -H "Authorization: Bearer $TOKEN" "${@:2}"; }
curl -s -X DELETE $URL/_localslack/faults   # start clean
```

Rule shape: `{"method":"chat.postMessage"|"*", "match":{"channel","app_id","user_id","token_kind"}?, "nth"?, "every"?, "times"?, "once"?, "action":{"status","error","retry_after","delay_ms","body"}}`.

- Counting: each live rule counts the calls that match its `method`/`match` (`seen`). A rule fires when `nth` (first eligible call, 1-based) / `every` (then every Nth) say so; `times` caps firings; `once` = `times:1`. `nth` alone = only that one call. A spent rule stops matching but stays listed (`spent:true`).
- The first firing rule in creation order wins. Faulted calls never run the real method.
- Checked before auth enforcement: a bad token is still faulted.

## Steps

1. 429 with Retry-After
   ```bash
   curl -s -X POST $URL/_localslack/faults -d '{"method":"auth.test","once":true,"action":{"status":429,"retry_after":2}}' | jq
   call auth.test
   call auth.test
   ```
2. 5xx and Slack-level errors
   ```bash
   curl -s -X POST $URL/_localslack/faults -d '{"method":"auth.test","once":true,"action":{"status":500}}' | jq
   curl -s -X POST $URL/_localslack/faults -d '{"method":"auth.test","once":true,"action":{"error":"fatal_error"}}' | jq
   call auth.test; call auth.test; call auth.test
   ```
3. nth: only the third call fails
   ```bash
   curl -s -X POST $URL/_localslack/faults -d '{"method":"auth.test","nth":3,"action":{"status":503}}' | jq
   for i in 1 2 3 4 5; do call auth.test | head -1; done
   curl -s $URL/_localslack/faults | jq '.items[] | {id,seen,fired,spent}'
   ```
4. every + times, wildcard, channel match
   ```bash
   curl -s -X POST $URL/_localslack/faults -d '{"method":"*","every":2,"times":2,"action":{"status":500}}' | jq .id
   curl -s -X POST $URL/_localslack/faults -d '{"method":"chat.postMessage","match":{"channel":"C123"},"action":{"error":"channel_not_found"}}' | jq .id
   ```
5. Slow call (timeout testing): delay alone runs the real method afterwards
   ```bash
   curl -s -X POST $URL/_localslack/faults -d '{"method":"auth.test","once":true,"action":{"delay_ms":3000}}' | jq
   time call auth.test | tail -1
   ```
6. Custom body: `{"method":"auth.test","action":{"status":502,"body":{"ok":false,"error":"bad_gateway"}}}`
7. Validation: `curl -s -i -X POST $URL/_localslack/faults -d '{"method":"auth.test","bogus":1,"action":{"status":200}}'`
8. Delete one / all: `curl -s -i -X DELETE $URL/_localslack/faults/f_1` (204; again -> 404 `not_found`), `curl -s -i -X DELETE $URL/_localslack/faults` (204).
9. With your own app (Bolt/SDK): add `{"method":"chat.postMessage","once":true,"action":{"status":429,"retry_after":2}}` and trigger a reply; the SDK `RateLimitErrorRetryHandler` should wait ~2s then succeed.
10. Reset (`localslack reset`) clears all faults (once stream C lands). With the call log available: `curl -s "$URL/_localslack/calls?method=auth.test" | jq '.items[] | {seq,status,error,fault_id}'` shows `fault_id` on faulted calls.

## Expected

- Step 1: first call `429`, header `Retry-After: 2`, body `{"ok":false,"error":"ratelimited"}`; second call `200`.
- Step 2: `500` `{"ok":false,"error":"internal_error"}`, then `200` with `{"ok":false,"error":"fatal_error"}`, then a normal `200 ok:true`.
- Step 3: statuses `200 200 503 200 200`; rule shows `seen:3, fired:1, spent:true` (spent rules stop counting).
- Step 4: wildcard fires on 2nd and 4th matching call, then is spent; the channel rule only affects `channel=C123`.
- Step 5: response takes ~3s and is the normal `ok:true` auth.test; the following call is fast.
- Step 6: `502` with exactly your body.
- Step 7: `422` `{"error":{"code":"invalid_fault","details":[{"pointer":"/bogus","message":"unknown field"}]}}`; drop `bogus` and it reports `/action` (status 200 alone does nothing). Malformed JSON gives `400 invalid_json`.
- Create returns `201` with `Location: /_localslack/faults/f_N` and `seen:0, fired:0`; list is `{"items":[...]}`.
- Limits: `delay_ms` <= 60000; `status` 200 or 400-599; 429 without `retry_after` sends `Retry-After: 1`.

## Feedback prompts

- Did `seen`/`fired` match what you expected for nth/every/times combos (esp. `nth` + `every`, shadowed rules)?
- Is `spent` rules staying in the list useful, or should they disappear?
- Does the SDK retry flow (`RateLimitErrorRetryHandler`, `ServerErrorRetryHandler`) behave like against real Slack? Any other Slack failure shapes you need (e.g. `Retry-After` on 200, `X-Slack-*` headers, connection drops)?
- Missing match fields (thread_ts, text substring)? Probabilistic faults (rate %) wanted, or is determinism enough?
