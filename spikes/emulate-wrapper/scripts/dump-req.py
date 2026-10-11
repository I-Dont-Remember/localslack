import asyncio, sys
from aiohttp import web
async def h(r):
    print(r.method, r.rel_url, dict(r.headers), await r.read(), flush=True); return web.json_response({"ok": True, "channel": {"id": "C1"}})
async def main():
    app = web.Application(); app.router.add_route("*", "/{p:.*}", h)
    run = web.AppRunner(app); await run.setup(); await web.TCPSite(run, "127.0.0.1", 5999).start()
    from slack_sdk.web.async_client import AsyncWebClient
    await AsyncWebClient(token="xoxb-t", base_url="http://127.0.0.1:5999/api/").conversations_create(name="as-1")
asyncio.run(main())
