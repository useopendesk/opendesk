// The slice of the Stripe API billing uses, in memory, after the Go fake in
// internal/billing/fake_stripe_test.go. Checkout answers with its own
// success_url so a browser comes straight back to the app.
// Run the API with STRIPE_API_URL=http://127.0.0.1:<port> STRIPE_SECRET_KEY=sk_test_x
// STRIPE_WEBHOOK_SECRET=whsec_x STRIPE_PRICE_PRO=price_pro STRIPE_PRICE_PRO_YEARLY=price_pro_y
// STRIPE_PRICE_AI_TOPUP=price_ai_topup (a one-time $10 AI top-up).
//
//   node fake-stripe.mjs [port]   (default 12111, on 127.0.0.1)

import http from "node:http";

const port = Number(process.argv[2] || 12111);
const prices = {
  price_pro: { id: "price_pro", object: "price", unit_amount: 1800, currency: "usd", recurring: { interval: "month" } },
  price_pro_y: { id: "price_pro_y", object: "price", unit_amount: 18000, currency: "usd", recurring: { interval: "year" } },
  price_ai_topup: { id: "price_ai_topup", object: "price", unit_amount: 1000, currency: "usd", recurring: null },
};
let n = 0;
const id = (p) => `${p}_${++n}${Math.random().toString(36).slice(2, 10)}`;

http
  .createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const u = new URL(req.url, "http://x");
      const form = new URLSearchParams(body);
      const send = (status, v) => res.writeHead(status, { "Content-Type": "application/json" }).end(JSON.stringify(v));
      const missing = () => send(404, { error: { type: "invalid_request_error", code: "resource_missing", message: "No such object" } });
      console.log(`${req.method} ${u.pathname}`);
      if (!String(req.headers.authorization).startsWith("Bearer sk_test_")) return send(401, { error: { type: "invalid_request_error", message: "bad key" } });
      const parts = u.pathname.split("/").filter(Boolean);
      if (req.method === "POST" && u.pathname === "/v1/customers") return send(200, { id: id("cus"), object: "customer" });
      if (req.method === "POST" && u.pathname === "/v1/checkout/sessions") return send(200, { id: id("cs"), object: "checkout.session", url: form.get("success_url") });
      if (req.method === "POST" && u.pathname === "/v1/billing_portal/sessions") return send(200, { id: id("bps"), object: "billing_portal.session", url: form.get("return_url") });
      if (req.method === "GET" && parts.length === 3 && parts[1] === "prices") return prices[parts[2]] ? send(200, prices[parts[2]]) : missing();
      if (req.method === "GET" && u.pathname === "/v1/subscriptions") return send(200, { object: "list", data: [], has_more: false, url: "/v1/subscriptions" });
      missing();
    });
  })
  .listen(port, "127.0.0.1", () => console.log(`fake stripe on http://127.0.0.1:${port}`));
