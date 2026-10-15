// A member's teams on the Members page: chips, and for admins a picker that
// adds or removes them (as a member, or an observer for viewers).

import { useQueryClient } from "@tanstack/react-query";
import { Pencil } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Picker } from "@/components/ui/picker";
import { toast } from "@/components/ui/toast";
import { api } from "@/lib/api/endpoints";
import type { Member, Team } from "@/lib/api/types";
import { qk, upsertInList } from "@/lib/queries";
import { TeamChip, teamItems, useTeams } from "./teams";

export function MemberTeams({ m, canEdit }: { m: Member; canEdit: boolean }) {
  const { t } = useTranslation("settingsTeams");
  const qc = useQueryClient();
  const { teams } = useTeams();
  if (!teams.length) return null;
  const on = teams.filter((x) => x.members.some((y) => y.user_id === m.user.id));

  const change = async (ids: string[]) => {
    const add = teams.filter((x) => ids.includes(x.id) && !on.includes(x));
    const drop = on.filter((x) => !ids.includes(x.id));
    const write = (x: Team, members: Team["members"]) => {
      upsertInList(qc, qk.teams, { ...x, members, member_ids: members.map((y) => y.user_id) });
      return api.teams.update(x.id, { members }).then((saved) => upsertInList(qc, qk.teams, saved));
    };
    try {
      await Promise.all([
        ...add.map((x) => write(x, [...x.members, { user_id: m.user.id, role: m.role === "viewer" ? "observer" : "member" }])),
        ...drop.map((x) => write(x, x.members.filter((y) => y.user_id !== m.user.id))),
      ]);
    } catch (e) {
      void qc.invalidateQueries({ queryKey: qk.teams });
      toast.error(e);
    }
  };

  return (
    <div className="mt-1 flex min-w-0 flex-wrap items-center gap-1">
      {on.map((x) => (
        <TeamChip key={x.id} team={x} size="xs" />
      ))}
      {canEdit && (
        <Picker
          multi
          values={on.map((x) => x.id)}
          onChange={(ids) => void change(ids)}
          items={teamItems(teams)}
          placeholder={t("common.searchTeams")}
          align="start"
          width={240}
          trigger={
            <button
                type="button"
                aria-label={t("scoped.editTeams", { name: m.user.name })}
                title={t("scoped.editTeams", { name: m.user.name })}
                className="inline-flex h-[18px] items-center gap-1 rounded-sm px-1 text-[10.5px] text-fg-4 transition-colors hover:bg-hover hover:text-fg-2"
              >
                <Pencil className="size-2.5" />
                {on.length ? null : t("scoped.addToTeam")}
              </button>
          }
        />
      )}
    </div>
  );
}
