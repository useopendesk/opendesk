// A mailbox's addressing (contracts/teams.md "Addresses"): its brand, its
// aliases and their teams, the From name customers see and the team its new tickets go to.
// Shared by the add-mailbox wizard and the mailbox page.

import { Lock } from "lucide-react";
import { useRef } from "react";
import { useTranslation } from "react-i18next";
import { ChipEditor } from "@/components/agent/ChipEditor";
import { BrandPick, Note, TeamPick, useBrands, useTeams } from "@/components/settings2/teams";
import { TextInput } from "@/components/ui/input";
import { useUser } from "@/lib/session";

export const FROM_VARS = ["{agent}", "{agent_first}", "{team}", "{brand}", "{mailbox}"] as const;
export const DEFAULT_FROM = "{agent} at {mailbox}";
const FROM_MAX = 80;
const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

type Vars = { agent: string; team: string; brand: string; mailbox: string };

/**
 * The From name for a format. Without an author (auto-replies, automations,
 * CSAT) the agent variable and the words up to the next variable are dropped,
 * a format left with no variable falls back to {mailbox}, and separators left
 * at either end are trimmed.
 */
export function fillFromName(format: string, v: Vars, withAgent: boolean): string {
  const parts = (format || DEFAULT_FROM).split(/(\{(?:agent|agent_first|team|brand|mailbox)\})/).filter((p) => p !== "");
  let kept = parts;
  if (!withAgent) {
    kept = [];
    for (let i = 0; i < parts.length; i++) {
      if (parts[i] === "{agent}" || parts[i] === "{agent_first}") {
        if (i + 1 < parts.length && !parts[i + 1].startsWith("{")) i++;
        continue;
      }
      kept.push(parts[i]);
    }
    if (!kept.some((p) => p.startsWith("{"))) kept = ["{mailbox}"];
  }
  const first = v.agent.split(/\s+/)[0] ?? v.agent;
  const out = kept
    .map((p) =>
      p === "{agent}" ? v.agent : p === "{agent_first}" ? first : p === "{team}" ? v.team || v.brand : p === "{brand}" ? v.brand : p === "{mailbox}" ? v.mailbox : p,
    )
    .join("");
  return out.replace(/^[\s\-–|·:,/]+|[\s\-–|·:,/]+$/g, "").replace(/\s+/g, " ");
}

export function fromNameProblem(format: string): "long" | "chars" | "variable" | null {
  if (format.length > FROM_MAX) return "long";
  if (/[<>"]/.test(format)) return "chars";
  const vars = format.match(/\{[^}]*\}/g) ?? [];
  if (vars.some((x) => !(FROM_VARS as readonly string[]).includes(x))) return "variable";
  return null;
}

/** The From name format with variable chips and a live preview, with and without an author. */
export function FromNameField({
  value,
  onChange,
  email,
  mailboxName,
  teamId,
  brandId,
}: {
  value: string;
  onChange: (v: string) => void;
  email: string;
  mailboxName: string;
  teamId: string | null;
  brandId: string | null | undefined;
}) {
  const { t } = useTranslation("settingsChannels");
  const me = useUser();
  const { teams } = useTeams();
  const { brands, defaultBrand } = useBrands();
  const ref = useRef<HTMLInputElement>(null);
  const brand = brands.find((b) => b.id === brandId) ?? defaultBrand;
  const vars: Vars = { agent: me.name || "Maya Chen", team: teams.find((x) => x.id === teamId)?.name ?? "", brand: brand?.name ?? "", mailbox: mailboxName || t("addressing.previewMailbox") };
  const problem = fromNameProblem(value);
  const insert = (v: string) => {
    const el = ref.current;
    const at = el?.selectionStart ?? value.length;
    const end = el?.selectionEnd ?? at;
    const next = value.slice(0, at) + v + value.slice(end);
    onChange(next);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(at + v.length, at + v.length);
    });
  };
  const addr = email || "support@example.com";
  return (
    <div className="flex flex-col gap-2">
      <TextInput
        ref={ref}
        aria-label={t("addressing.fromName")}
        value={value}
        maxLength={FROM_MAX + 10}
        invalid={!!problem}
        placeholder={DEFAULT_FROM}
        onChange={(e) => onChange(e.target.value)}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        {FROM_VARS.map((v) => (
          <button
            key={v}
            type="button"
            onClick={() => insert(v)}
            className="mono inline-flex h-[22px] items-center rounded-sm border border-line bg-sunken px-1.5 text-[11px] text-fg-2 transition-colors hover:border-line-strong hover:text-fg"
            title={t(`addressing.vars.${v.slice(1, -1) as "agent"}`)}
          >
            {v}
          </button>
        ))}
      </div>
      {problem ? (
        <p role="alert" className="text-[12px] text-danger-fg">
          {t(`addressing.fromErrors.${problem}`, { max: FROM_MAX })}
        </p>
      ) : (
        <div className="flex flex-col gap-1 rounded-md border border-line bg-sunken/60 px-3 py-2">
          <PreviewLine label={t("addressing.previewReply")} name={fillFromName(value, vars, true)} email={addr} />
          <PreviewLine label={t("addressing.previewAuto")} name={fillFromName(value, vars, false)} email={addr} />
        </div>
      )}
    </div>
  );
}

function PreviewLine({ label, name, email }: { label: string; name: string; email: string }) {
  return (
    <div className="flex min-w-0 items-baseline gap-2 text-[12px]">
      <span className="w-[92px] shrink-0 text-fg-4">{label}</span>
      <span className="min-w-0 truncate">
        <span className="font-medium text-fg">{name}</span> <span className="mono text-[11.5px] text-fg-3">&lt;{email}&gt;</span>
      </span>
    </div>
  );
}

/** Other addresses that deliver into this mailbox. */
export function AliasesField({ value, onChange, email }: { value: string[]; onChange: (v: string[]) => void; email: string }) {
  const { t } = useTranslation("settingsChannels");
  return (
    <ChipEditor
      mono
      values={value}
      onChange={onChange}
      max={20}
      normalize={(v) => {
        const x = v.trim().toLowerCase();
        return emailRe.test(x) ? x : null;
      }}
      validate={(v) => (v === email.trim().toLowerCase() ? t("addressing.aliasSame") : null)}
      placeholder="help@company.com"
      aria-label={t("addressing.aliases")}
      empty={t("addressing.noAliases")}
    />
  );
}

/**
 * The alias teams to save: only aliases that still exist, and none on a private
 * team's address (it delivers every alias to that team).
 */
export function aliasTeamsFor(value: Record<string, string>, aliases: string[], teamPrivate: boolean): Record<string, string> {
  if (teamPrivate) return {};
  return Object.fromEntries(aliases.filter((a) => value[a]).map((a) => [a, value[a]]));
}

export function sameAliasTeams(a: Record<string, string>, b: Record<string, string>) {
  const ka = Object.keys(a);
  return ka.length === Object.keys(b).length && ka.every((k) => a[k] === b[k]);
}

/** Where new tickets to each alias go: the mailbox's team, or one of their own. */
export function AliasTeamsField({
  aliases,
  value,
  onChange,
  teamId,
}: {
  aliases: string[];
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  /** The mailbox's own team. */
  teamId: string | null;
}) {
  const { t } = useTranslation("settingsChannels");
  const { teams } = useTeams();
  if (!aliases.length || !teams.length) return null;
  const team = teams.find((x) => x.id === teamId);
  if (team?.private) {
    return (
      <Note tone="violet" icon={<Lock className="mt-0.5 size-3.5 shrink-0" aria-hidden />}>
        {t("addressing.aliasTeamsPrivate", { name: team.name })}
      </Note>
    );
  }
  const set = (alias: string, id: string | null) => {
    const next = { ...value };
    if (id && id !== teamId) next[alias] = id;
    else delete next[alias];
    onChange(next);
  };
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[12px] text-fg-3">{t("addressing.aliasTeams")}</span>
      <ul className="flex flex-col divide-y divide-line rounded-md border border-line">
        {aliases.map((a) => (
          <li key={a} className="flex items-center gap-3 px-3 py-1.5">
            <span className="mono min-w-0 flex-1 truncate text-[12px] text-fg-2" title={a}>
              {a}
            </span>
            <TeamPick
              className="w-[220px]"
              value={value[a] ?? null}
              noneLabel={team ? t("addressing.aliasTeamSame", { name: team.name }) : t("addressing.aliasTeamNone")}
              onChange={(id) => set(a, id)}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

/** "New tickets to this address go to", with a note when the team is private. */
export function MailboxTeamField({ value, onChange, className }: { value: string | null; onChange: (id: string | null) => void; className?: string }) {
  const { t } = useTranslation("settingsChannels");
  const { teams } = useTeams();
  const team = teams.find((x) => x.id === value);
  return (
    <div className="flex flex-col gap-2">
      <TeamPick className={className} value={value} noneLabel={t("addressing.noTeam")} onChange={onChange} />
      {team?.private && (
        <Note tone="violet" icon={<Lock className="mt-0.5 size-3.5 shrink-0" aria-hidden />}>
          {t("addressing.privateTeam", { name: team.name })}
        </Note>
      )}
    </div>
  );
}

/** The brand picker, shown only once there is a second brand. */
export function MailboxBrandField({ value, onChange, className }: { value: string | null | undefined; onChange: (id: string) => void; className?: string }) {
  const { multi, defaultBrand } = useBrands();
  if (!multi) return null;
  return <BrandPick className={className} value={value || defaultBrand?.id} onChange={onChange} />;
}
