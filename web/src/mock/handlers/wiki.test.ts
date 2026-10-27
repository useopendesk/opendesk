import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import type { WikiNode, WikiPage, WikiRevision, WikiSearchHit } from "@/lib/api/types";
import { qk } from "@/lib/queries";
import { applyWikiEvent } from "@/lib/wiki/events";
import { installMock } from "../index";
import { dispatch } from "../router";
import { db } from "../db";

async function call(method: string, path: string, body?: unknown, wid?: string) {
  const ws = wid ?? db.workspaces[0]?.workspace.id;
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined, headers: ws ? { "OD-Workspace": ws } : {} });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

describe("mock wiki", () => {
  installMock();

  it("seeds a handbook in the first workspace and nothing elsewhere", async () => {
    const tree = (await call("GET", "/v1/wiki/pages")).body.data as WikiNode[];
    expect(tree.find((n) => n.title === "Support handbook")).toBeTruthy();
    const lumen = db.workspaces[1].workspace.id;
    expect((await call("GET", "/v1/wiki/pages", undefined, lumen)).body.data).toEqual([]);
  });

  it("saves with versions, refuses a stale one and keeps one revision per burst of typing", async () => {
    const p = (await call("POST", "/v1/wiki/pages", { title: "Refunds" })).body as WikiPage;
    expect(p.version).toBe(1);
    const a = (await call("PATCH", `/v1/wiki/pages/${p.id}`, { body_html: "<p>a</p>", version: 1 })).body as WikiPage;
    const b = (await call("PATCH", `/v1/wiki/pages/${p.id}`, { body_html: "<p>ab</p>", version: a.version })).body as WikiPage;
    expect(b.version).toBe(3);
    const stale = await call("PATCH", `/v1/wiki/pages/${p.id}`, { body_html: "<p>old</p>", version: 1 });
    expect(stale).toMatchObject({ status: 409, body: { code: "page_changed" } });
    const revs = (await call("GET", `/v1/wiki/pages/${p.id}/revisions`)).body.data as WikiRevision[];
    expect(revs).toHaveLength(1);
  });

  it("moves without cycles and deletes subpages with their page", async () => {
    const top = (await call("POST", "/v1/wiki/pages", { title: "Top" })).body as WikiPage;
    const sub = (await call("POST", "/v1/wiki/pages", { title: "Sub", parent_id: top.id })).body as WikiPage;
    expect(sub.breadcrumbs.map((c) => c.id)).toEqual([top.id]);
    expect((await call("POST", `/v1/wiki/pages/${top.id}/move`, { parent_id: sub.id, position: 0 })).body.code).toBe("page_cycle");
    expect((await call("DELETE", `/v1/wiki/pages/${top.id}`)).status).toBe(204);
    expect((await call("GET", `/v1/wiki/pages/${sub.id}`)).status).toBe(404);
  });

  it("finds pages by partly typed words, in global search too", async () => {
    const hits = (await call("GET", "/v1/wiki/search?q=escal")).body.data as WikiSearchHit[];
    expect(hits[0]?.title).toBe("Escalation process");
    expect(hits[0]?.path.map((p) => p.title)).toEqual(["Support handbook"]);
    const all = (await call("GET", "/v1/search?q=escalation")).body;
    expect(all.wiki_pages.length).toBeGreaterThan(0);
  });
});

describe("wiki events on the cache", () => {
  const node = (id: string, parent: string | null, position: number, version = 1): WikiNode => ({ id, parent_id: parent, title: id, icon: "", position, version, updated_at: "2026-10-01T00:00:00Z" });

  it("patches the tree, refetches a page someone else changed and drops deleted subpages", () => {
    const qc = new QueryClient();
    qc.setQueryData(qk.wikiTree, [node("a", null, 0), node("b", "a", 0)]);
    qc.setQueryData(qk.wikiPage("a"), { ...node("a", null, 0), body_html: "" });
    applyWikiEvent(qc, "created", { wiki_page: node("c", null, 1), updated_by: null }, "me");
    expect(qc.getQueryData<WikiNode[]>(qk.wikiTree)?.map((n) => n.id)).toEqual(["a", "b", "c"]);
    applyWikiEvent(qc, "updated", { wiki_page: { ...node("a", null, 0, 2), title: "Renamed" }, updated_by: { id: "other" } }, "me");
    expect(qc.getQueryData<WikiNode[]>(qk.wikiTree)?.[0]?.title).toBe("Renamed");
    expect(qc.getQueryState(qk.wikiPage("a"))?.isInvalidated).toBe(true);
    applyWikiEvent(qc, "deleted", { wiki_page_id: "a", wiki_page_ids: ["a", "b"] }, "me");
    expect(qc.getQueryData<WikiNode[]>(qk.wikiTree)).toEqual([node("c", null, 0)]);
    expect(qc.getQueryData(qk.wikiPage("a"))).toBeUndefined();
  });
});
