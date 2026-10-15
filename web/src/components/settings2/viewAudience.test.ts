import { describe, expect, it } from "vitest";
import { i18n } from "@/i18n";
import { viewAudience, viewAudienceHint, viewDeleteBody } from "./viewAudience";

const t = i18n.getFixedT("en", "settingsInbox");

describe("view audience", () => {
  it("reads the audience from the view", () => {
    expect(viewAudience({ shared: true, team_id: null })).toBe("everyone");
    expect(viewAudience({ shared: false })).toBe("me");
    expect(viewAudience({ shared: false, team_id: "t1" })).toBe("team");
    expect(viewAudience({ shared: true, team_id: "t1" })).toBe("team");
  });

  it("describes the chosen option, not the switch", () => {
    expect(viewAudienceHint(t, "everyone")).toBe("Everyone in the workspace sees it in their sidebar.");
    expect(viewAudienceHint(t, "me")).toBe("Only you see it in your sidebar.");
    expect(viewAudienceHint(t, "team", "Billing")).toBe("Everyone on Billing sees it in their sidebar.");
    expect(viewAudienceHint(t, "team", null)).toBe("Everyone on the team sees it in their sidebar.");
    for (const a of ["everyone", "me", "team"] as const) expect(viewAudienceHint(t, a)).not.toMatch(/^Off /);
  });

  it("says whose sidebar loses a deleted view", () => {
    expect(viewDeleteBody(t, "me")).toBe("will disappear from your sidebar. Tickets are not affected.");
    expect(viewDeleteBody(t, "team", "Legal")).toBe("will disappear from the sidebar of everyone on Legal. Tickets are not affected.");
    expect(viewDeleteBody(t, "everyone")).toBe("will disappear from the sidebar for everyone. Tickets are not affected.");
  });
});
