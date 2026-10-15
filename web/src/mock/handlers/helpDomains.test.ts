import { describe, expect, it } from "vitest";
import type { Brand } from "@/lib/api/types";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function call(method: string, path: string, body?: unknown) {
  const ws = db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

const lastBrandEvent = () => {
  const e = [...db.workspaces[0].events].reverse().find((x) => x.type === "brand.updated");
  return (e?.data as { brand: Brand } | undefined)?.brand;
};

describe("mock help center domains", () => {
  installMock();

  it("gives every brand a default address and no domain", async () => {
    const brands = (await call("GET", "/v1/brands")).body.data as Brand[];
    for (const b of brands) {
      expect(b.help_center.domain).toBeNull();
      expect(b.help_center.default_url).toMatch(new RegExp(`/help/${b.slug}$|//${b.slug}\\.`));
      expect(b.help_center.url).toBe(b.help_center.default_url);
    }
  });

  it("connects a domain, checks it to active, and removes it", async () => {
    const brand = ((await call("GET", "/v1/brands")).body.data as Brand[]).find((b) => b.slug === "pressroom")!;
    const base = `/v1/brands/${brand.id}/help-domain`;
    expect((await call("PUT", base, { hostname: "pressroom.app" })).body.code).toBe("hostname_invalid");
    expect((await call("PUT", base, { hostname: "https://help.pressroom.app" })).status).toBe(422);
    expect((await call("POST", `${base}/check`)).status).toBe(404);

    const set = (await call("PUT", base, { hostname: "Help.Pressroom.app" })).body as Brand;
    const d = set.help_center.domain!;
    expect(d).toMatchObject({ hostname: "help.pressroom.app", status: "pending", problem: "txt_missing", verified_at: null });
    expect(d.records.map((r) => [r.type, r.name, r.ok])).toEqual([
      ["TXT", "_opendesk-verify.help.pressroom.app", false],
      ["CNAME", "help.pressroom.app", false],
    ]);
    expect(d.records[0].value).toMatch(/^opendesk-verify=/);
    expect(set.help_center.url).toBe(set.help_center.default_url);
    expect(lastBrandEvent()?.help_center.domain?.hostname).toBe("help.pressroom.app");

    const steps: string[] = [];
    let b = set;
    for (let i = 0; i < 3; i++) {
      b = (await call("POST", `${base}/check`)).body as Brand;
      steps.push(`${b.help_center.domain!.status}:${b.help_center.domain!.problem}`);
    }
    expect(steps).toEqual(["pending:cname_missing", "pending:certificate_pending", "active:"]);
    expect(b.help_center.url).toBe("https://help.pressroom.app");
    expect(b.help_center.domain!.records.every((r) => r.ok)).toBe(true);
    expect(lastBrandEvent()?.help_center.domain?.status).toBe("active");

    // Another brand can't take a verified domain.
    const other = ((await call("GET", "/v1/brands")).body.data as Brand[]).find((x) => x.id !== brand.id)!;
    const taken = await call("PUT", `/v1/brands/${other.id}/help-domain`, { hostname: "help.pressroom.app" });
    expect([taken.status, taken.body.code]).toEqual([409, "domain_taken"]);

    expect((await call("DELETE", base)).status).toBe(204);
    const after = lastBrandEvent()!;
    expect(after.help_center.domain).toBeNull();
    expect(after.help_center.url).toBe(after.help_center.default_url);
  });

  it("limits checks to six a minute per brand", async () => {
    const brand = ((await call("GET", "/v1/brands")).body.data as Brand[]).find((b) => b.slug === "pressroom")!;
    const base = `/v1/brands/${brand.id}/help-domain`;
    await call("PUT", base, { hostname: "docs.nodns.example.com" });
    const codes: number[] = [];
    for (let i = 0; i < 7; i++) codes.push((await call("POST", `${base}/check`)).status);
    expect(codes).toEqual([200, 200, 200, 200, 200, 200, 429]);
    await call("DELETE", base);
  });
});
