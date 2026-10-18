// opendesk-app (app.opendesk.sh). Copied into web/dist by deploy.yml with
// _routes.json, which sends only /assets/* here; every other path is served by
// Pages as before (static files, the SPA fallback, _headers, _redirects).
//
// A file under /assets/ answers only as itself: one the deployment does not have
// is a 404 that nothing caches, never the SPA's index.html, which the "immutable"
// rule for /assets/* would keep in browsers for a year under a script's name.
// Mirrors internal/webui.
export default {
  async fetch(request, env) {
    const res = await env.ASSETS.fetch(request);
    if (res.ok && (res.headers.get("Content-Type") ?? "").startsWith("text/html")) {
      return new Response("Not found\n", {
        status: 404,
        headers: {
          "Content-Type": "text/plain; charset=utf-8",
          "Cache-Control": "no-store",
          "X-Content-Type-Options": "nosniff",
        },
      });
    }
    return res;
  },
};
