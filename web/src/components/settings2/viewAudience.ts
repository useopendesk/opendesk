// Who a saved view is for: only its owner, one team, or everyone in the
// workspace (contracts/teams.md). The copy follows the chosen audience.

import type { TFunction } from "i18next";

export type ViewAudience = "me" | "team" | "everyone";

/** A team view is never shared with everyone; otherwise `shared` decides. */
export function viewAudience(v: { shared: boolean; team_id?: string | null }): ViewAudience {
  return v.team_id ? "team" : v.shared ? "everyone" : "me";
}

/** Who sees the view in their sidebar, naming the team when it is known. */
export function viewAudienceHint(t: TFunction<"settingsInbox">, audience: ViewAudience, teamName?: string | null): string {
  if (audience === "team") return teamName ? t("views.audience.team", { team: teamName }) : t("views.audience.anyTeam");
  return audience === "everyone" ? t("views.audience.everyone") : t("views.audience.me");
}

/** What deleting the view takes away, and from whom (follows the bold view name). */
export function viewDeleteBody(t: TFunction<"settingsInbox">, audience: ViewAudience, teamName?: string | null): string {
  if (audience === "team") return teamName ? t("views.deleteFrom.team", { team: teamName }) : t("views.deleteFrom.anyTeam");
  return audience === "everyone" ? t("views.deleteFrom.everyone") : t("views.deleteFrom.me");
}
