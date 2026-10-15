import { beforeAll, describe, expect, it } from "vitest";
import type { ApiKey, Team, Webhook } from "@/lib/api/types";
import { installMock } from "./index";
import { dispatch } from "./router";

async function call(method: string, path: string, body?: unknown) {
  const r = await dispatch(`http://mock${path}`, { method, body: body ? JSON.stringify(body) : undefined });
  return { status: r.status, body: r.status === 204 ? null : await r.json() };
}

const list = async <T,>(path: string) => (await call("GET", path)).body.data as T[];

// contracts/teams.md "Visibility": a key's or webhook's private team lasts while its granter is on the team.
describe("mock private team grants", () => {
  beforeAll(() => {
    localStorage.clear();
    installMock();
  });

  it("lists who granted each team", async () => {
    const key = (await list<ApiKey>("/v1/api-keys")).find((k) => k.name === "Legal hold archive")!;
    expect(key.private_teams?.map((g) => [g.name, g.granted_by?.name.split(" ")[0]])).toEqual([["Legal", "Jonas"]]);
    const hook = (await list<Webhook>("/v1/webhooks")).find((h) => h.private_team_ids?.length)!;
    expect(hook.private_teams?.map((g) => g.name)).toEqual(["Security"]);
  });

  it("drops a grant when its granter is taken off the team or out of the workspace", async () => {
    const teams = await list<Team>("/v1/teams");
    const legal = teams.find((t) => t.name === "Legal")!;
    const jonas = (await list<ApiKey>("/v1/api-keys")).find((k) => k.name === "Legal hold archive")!.private_teams![0].granted_by!.id;
    const rest = legal.members.filter((m) => m.user_id !== jonas);
    expect((await call("PATCH", `/v1/teams/${legal.id}`, { members: rest })).status).toBe(200);
    const key = (await list<ApiKey>("/v1/api-keys")).find((k) => k.name === "Legal hold archive")!;
    expect(key.private_team_ids).toEqual([]);
    expect(key.private_teams).toEqual([]);

    const hook = (await list<Webhook>("/v1/webhooks")).find((h) => h.private_team_ids?.length)!;
    const priya = hook.private_teams![0].granted_by!.id;
    expect((await call("DELETE", `/v1/members/${priya}`)).status).toBe(204);
    const after = (await list<Webhook>("/v1/webhooks")).find((h) => h.id === hook.id)!;
    expect(after.private_team_ids).toEqual([]);
  });
});
