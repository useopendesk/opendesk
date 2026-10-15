// Settings > Teams: who owns which work (contracts/teams.md). The list shows
// each team's colour, privacy, people, open work and addresses; a team opens
// on its own page.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { ChevronRight, Inbox, Lock, Plus, UsersRound } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Section, SettingsPage } from "@/components/layout";
import { ListSkeleton } from "@/components/settings2/bits";
import { ColorSwatchPicker } from "@/components/settings2/ColorSwatchPicker";
import { MotionList, MotionRow } from "@/components/settings2/motion";
import { SettingsEmpty } from "@/components/settings2/SettingsEmpty";
import { PrivatePill, TeamDot } from "@/components/settings2/teams";
import { AvatarStack } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Textarea, TextInput } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { toast } from "@/components/ui/toast";
import { Tooltip } from "@/components/ui/tooltip";
import { api } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import type { LabelColor, Member, Team } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { q, qk, upsertInList } from "@/lib/queries";
import { atLeast, useRole, useUser } from "@/lib/session";
import { tone } from "@/lib/tones";

export default function Teams() {
  const { t } = useTranslation("settingsTeams");
  const admin = atLeast(useRole(), "admin");
  const { data: teams, isLoading } = useQuery(q.teams());
  const { data: members } = useQuery(q.members());
  const { data: mailboxes } = useQuery({ ...q.mailboxes(), enabled: admin });
  const [creating, setCreating] = useState(false);
  const byId = new Map((members ?? []).map((m) => [m.user.id, m]));

  const newButton = (variant: "primary" | "secondary" = "primary") =>
    admin && (
      <Button variant={variant} icon={<Plus className="size-3.5" />} onClick={() => setCreating(true)}>
        {t("list.new")}
      </Button>
    );

  return (
    <SettingsPage title={t("list.title")} description={t("list.description")} actions={teams?.length ? newButton() : undefined}>
      {isLoading ? (
        <Section flush>
          <ListSkeleton rows={4} />
        </Section>
      ) : !teams?.length ? (
        <Section flush>
          <SettingsEmpty
            art="contacts"
            ghost="people"
            title={t("list.emptyTitle")}
            description={t("list.emptyHint")}
            action={newButton()}
            tips={[
              { label: t("list.tips.address"), icon: <Inbox /> },
              { label: t("list.tips.private"), icon: <Lock /> },
            ]}
          />
        </Section>
      ) : (
        <Section flush title={t("list.count", { count: teams.length })}>
          <div className="hidden h-8 items-center gap-3 border-b border-line-soft bg-sunken/50 px-4 text-[11px] font-medium text-fg-4 md:flex">
            <span className="flex-1">{t("list.colTeam")}</span>
            <span className="w-[120px]">{t("list.colPeople")}</span>
            <span className="w-[64px] text-right">{t("list.colOpen")}</span>
            <span className="w-[84px] text-right">{t("list.colUnassigned")}</span>
            <span className="w-4" />
          </div>
          <MotionList>
            {teams.map((team) => (
              <MotionRow key={team.id}>
                <TeamRow team={team} byId={byId} addresses={(mailboxes ?? []).filter((m) => m.team_id === team.id).map((m) => m.email)} />
              </MotionRow>
            ))}
          </MotionList>
        </Section>
      )}
      {creating && <NewTeamDialog onClose={() => setCreating(false)} />}
    </SettingsPage>
  );
}

function TeamRow({ team, byId, addresses }: { team: Team; byId: Map<string, Member>; addresses: string[] }) {
  const { t } = useTranslation("settingsTeams");
  const me = useUser();
  const people = team.members.map((m) => byId.get(m.user_id)).filter((m): m is Member => !!m);
  const mine = team.members.some((m) => m.user_id === me.id);
  const hidden = team.private && !team.members.length && !mine;
  const shown = addresses.slice(0, 2);
  return (
    <Link
      to="/settings/teams/$teamId"
      params={{ teamId: team.id }}
      className="group flex min-h-14 items-center gap-3 border-b border-line-soft px-4 py-2.5 outline-none transition-colors last:border-0 hover:bg-sunken focus-visible:bg-sunken"
    >
      <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-md", tone[team.color]?.soft ?? tone.gray.soft)}>
        <TeamDot team={team} className="size-2.5" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-[13px] font-medium">{team.name}</span>
          {team.private && <PrivatePill />}
          {mine && <span className="text-[11px] text-fg-4">{t("list.youreOn")}</span>}
        </div>
        <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[12px] text-fg-3">
          {shown.length ? (
            <>
              <Inbox className="size-3 shrink-0 text-fg-4" aria-hidden />
              <span className="mono truncate text-[11.5px]">
                {shown.join(", ")}
                {addresses.length > shown.length && <span className="text-fg-4"> {t("list.more", { count: addresses.length - shown.length })}</span>}
              </span>
            </>
          ) : (
            <span className="truncate">{team.description || t("list.noAddress")}</span>
          )}
        </div>
      </div>
      <div className="hidden w-[120px] items-center md:flex">
        {hidden ? (
          <Tooltip content={t("list.hiddenMembers")}>
            <span className="inline-flex items-center gap-1 text-[12px] text-fg-4">
              <Lock className="size-3" aria-hidden />
              {t("list.membersCount", { count: team.member_ids.length })}
            </span>
          </Tooltip>
        ) : people.length ? (
          <AvatarStack people={people.map((m) => ({ id: m.user.id, name: m.user.name, avatar: m.user.avatar_url }))} max={4} size="md" />
        ) : (
          <span className="text-[12px] text-fg-4">{t("list.noMembers")}</span>
        )}
      </div>
      <span className={cn("hidden w-[64px] text-right text-[13px] tabular-nums md:block", team.open_count ? "text-fg" : "text-fg-4")}>{team.open_count}</span>
      <span className={cn("hidden w-[84px] text-right text-[13px] tabular-nums md:block", team.unassigned_count ? tone.amber.text : "text-fg-4")}>{team.unassigned_count}</span>
      <ChevronRight className="size-4 shrink-0 text-fg-4 transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none" aria-hidden />
    </Link>
  );
}

function NewTeamDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("settingsTeams");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const me = useUser();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [color, setColor] = useState<LabelColor>("blue");
  const [priv, setPriv] = useState(false);
  const [join, setJoin] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () =>
      api.teams.create({
        name: name.trim(),
        description: description.trim(),
        color,
        private: priv,
        members: join ? [{ user_id: me.id, role: "lead" }] : [],
      }),
    onSuccess: (team) => {
      upsertInList(qc, qk.teams, team);
      toast.success(t("list.created", { name: team.name }));
      onClose();
      void navigate({ to: "/settings/teams/$teamId", params: { teamId: team.id } });
    },
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("list.new")}
      description={t("list.newHint")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button variant="primary" type="submit" form="new-team" loading={create.isPending} icon={<UsersRound className="size-3.5" />}>
            {t("list.create")}
          </Button>
        </>
      }
    >
      <form
        id="new-team"
        className="flex flex-col gap-4 pb-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!name.trim()) return setError(t("general.nameRequired"));
          create.mutate();
        }}
      >
        <Field label={t("general.name")} error={error}>
          {({ id, invalid }) => (
            <TextInput id={id} autoFocus invalid={invalid} value={name} maxLength={60} placeholder={t("general.namePlaceholder")} onChange={(e) => setName(e.target.value)} />
          )}
        </Field>
        <Field label={t("general.description")} optional>
          {({ id }) => (
            <Textarea id={id} rows={2} value={description} maxLength={280} placeholder={t("general.descriptionPlaceholder")} onChange={(e) => setDescription(e.target.value)} />
          )}
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[12.5px] font-medium">{t("general.color")}</span>
          <ColorSwatchPicker value={color} onChange={(c) => setColor(c)} label={t("general.color")} size="sm" />
        </div>
        <div className="flex flex-col rounded-md border border-line">
          <label className="flex items-center justify-between gap-3 border-b border-line-soft px-3 py-2.5">
            <span className="min-w-0">
              <span className="block text-[12.5px] font-medium">{t("list.joinAsLead")}</span>
              <span className="block text-[12px] text-fg-3">{t("list.joinAsLeadHint")}</span>
            </span>
            <Switch checked={join} onCheckedChange={setJoin} aria-label={t("list.joinAsLead")} />
          </label>
          <label className="flex items-center justify-between gap-3 px-3 py-2.5">
            <span className="min-w-0">
              <span className="flex items-center gap-1.5 text-[12.5px] font-medium">
                <Lock className="size-3 text-fg-3" aria-hidden />
                {t("privacy.label")}
              </span>
              <span className="block text-[12px] text-fg-3">{t("list.privateHint")}</span>
            </span>
            <Switch checked={priv} onCheckedChange={setPriv} aria-label={t("privacy.label")} />
          </label>
        </div>
      </form>
    </Dialog>
  );
}
