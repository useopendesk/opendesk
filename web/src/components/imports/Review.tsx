// What a check found: the account, how much it holds, a few of its tickets as they will
// look here, and who and which groups they map to.

import { ArrowUpRight, Check, ChevronLeft, ChevronRight, Lock, Paperclip } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Avatar } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Select } from "@/components/ui/select";
import type { ImportCheck, ImportSample, ImportSource, Member, Team } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { number, shortDate } from "@/lib/format";
import { dur, ease } from "@/lib/motion";
import { statusTone } from "@/lib/tones";
import { SourceMark } from "./sources";

const NONE = "none";

/** The account and its counts, as one calm card. */
export function AccountCard({ source, check }: { source: ImportSource; check: ImportCheck }) {
  const { t } = useTranslation("imports");
  const parts = [
    check.totals.tickets !== null ? t("review.tickets", { count: check.totals.tickets, n: number(check.totals.tickets) }) : t("review.ticketsUnknown"),
    check.has_articles && check.totals.articles !== null ? t("review.articles", { count: check.totals.articles, n: number(check.totals.articles) }) : null,
    check.agents.length ? t("review.agents", { count: check.agents.length }) : null,
  ].filter(Boolean);
  return (
    <div className="flex items-center gap-3 rounded-lg border border-line bg-surface px-4 py-3 shadow-xs">
      <SourceMark source={source} size="lg" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-[14px] font-semibold tracking-[-0.01em] text-fg">{check.account.name}</span>
          <Badge tone="ok">{source === "csv" ? t("review.ready") : t("review.connected")}</Badge>
        </div>
        <div className="mt-0.5 truncate text-[12px] text-fg-3">{parts.join(" · ")}</div>
      </div>
      {check.account.url && (
        <a
          href={check.account.url}
          target="_blank"
          rel="noopener noreferrer"
          className="mono hidden shrink-0 items-center gap-0.5 text-[11.5px] text-fg-4 transition-colors hover:text-fg sm:inline-flex"
        >
          {check.account.url.replace(/^https?:\/\//, "")}
          <ArrowUpRight className="size-3" aria-hidden />
        </a>
      )}
    </div>
  );
}

/** One sample ticket as a short conversation, with arrows between samples. */
export function SamplePreview({ samples, title }: { samples: ImportSample[]; title?: string }) {
  const { t } = useTranslation("imports");
  const reduce = useReducedMotion();
  const [i, setI] = useState(0);
  if (!samples.length) return <p className="rounded-lg border border-dashed border-line px-4 py-6 text-center text-[12.5px] text-fg-3">{t("review.noSamples")}</p>;
  const s = samples[Math.min(i, samples.length - 1)]!;
  const btn = "flex size-6 items-center justify-center rounded-md text-fg-3 transition-colors hover:bg-hover hover:text-fg disabled:pointer-events-none disabled:opacity-30";
  return (
    <div className="overflow-hidden rounded-lg border border-line bg-surface shadow-xs">
      <div className="flex items-center gap-2 border-b border-line-soft bg-sunken px-3 py-1.5">
        <span className="label-caps">{title ?? t("review.sampleTitle")}</span>
        {samples.length > 1 && (
          <div className="ml-auto flex items-center gap-0.5">
            <span className="mono mr-1 text-[10.5px] tabular-nums text-fg-4">
              {i + 1}/{samples.length}
            </span>
            <button type="button" className={btn} aria-label={t("review.previous")} disabled={i === 0} onClick={() => setI(i - 1)}>
              <ChevronLeft className="size-3.5" />
            </button>
            <button type="button" className={btn} aria-label={t("review.next")} disabled={i >= samples.length - 1} onClick={() => setI(i + 1)}>
              <ChevronRight className="size-3.5" />
            </button>
          </div>
        )}
      </div>
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={i}
          initial={reduce ? false : { opacity: 0, x: 6 }}
          animate={{ opacity: 1, x: 0 }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, x: -6 }}
          transition={{ duration: dur.base, ease: ease.out }}
          className="px-4 py-3"
        >
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[13.5px] font-semibold tracking-[-0.01em] text-fg">{s.subject || t("review.noSubject")}</div>
              <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[11.5px] text-fg-3">
                {s.ref && <span className="mono text-fg-4">{s.ref}</span>}
                {s.requester && <span>{s.requester.name || s.requester.email}</span>}
                <span className="text-fg-4">{shortDate(s.created_at)}</span>
                <span className="text-fg-4">{t("review.messages", { count: s.message_count })}</span>
              </div>
            </div>
            <Badge tone={statusTone[s.status]}>{t(`status.${s.status}`)}</Badge>
          </div>
          {s.tags.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1">
              {s.tags.slice(0, 6).map((g) => (
                <Badge key={g} tone="outline">
                  {g}
                </Badge>
              ))}
            </div>
          )}
          <ol className="mt-3 flex flex-col gap-2">
            {s.messages.map((m, j) => (
              <li key={j} className={cn("flex gap-2.5", m.agent && !m.note && "flex-row-reverse")}>
                <Avatar name={m.author || "?"} size="sm" />
                <div
                  className={cn(
                    "min-w-0 max-w-[82%] rounded-lg px-3 py-2 text-[12.5px] leading-[18px]",
                    m.note ? "bg-tone-amber-soft text-fg shadow-[inset_0_0_0_1px_var(--tone-amber-line)]" : m.agent ? "bg-hover text-fg" : "border border-line bg-surface text-fg",
                  )}
                >
                  <div className="mb-0.5 flex items-center gap-1.5 text-[11px] text-fg-3">
                    <span className="truncate font-medium text-fg-2">{m.author || t("review.unknown")}</span>
                    {m.note && (
                      <span className="inline-flex items-center gap-0.5 text-tone-amber">
                        <Lock className="size-2.5" aria-hidden />
                        {t("review.note")}
                      </span>
                    )}
                    {m.attachments > 0 && (
                      <span className="inline-flex items-center gap-0.5">
                        <Paperclip className="size-2.5" aria-hidden />
                        {m.attachments}
                      </span>
                    )}
                  </div>
                  <p className="line-clamp-4 whitespace-pre-line break-words">{m.text || <span className="text-fg-4">{t("review.empty")}</span>}</p>
                </div>
              </li>
            ))}
          </ol>
          {s.message_count > s.messages.length && <p className="mt-2 text-center text-[11.5px] text-fg-4">{t("review.more", { count: s.message_count - s.messages.length })}</p>}
        </motion.div>
      </AnimatePresence>
    </div>
  );
}

/** Each source agent onto a member, each group onto a team. Matches by email and name come preset. */
export function MappingTable({
  check,
  members,
  teams,
  agents,
  groups,
  onAgents,
  onGroups,
}: {
  check: ImportCheck;
  members: Member[];
  teams: Team[];
  agents: Record<string, string | null>;
  groups: Record<string, string | null>;
  onAgents: (v: Record<string, string | null>) => void;
  onGroups: (v: Record<string, string | null>) => void;
}) {
  const { t } = useTranslation("imports");
  const memberOptions = [{ value: NONE, label: t("review.keepName") }, ...members.map((m) => ({ value: m.user.id, label: m.user.name, hint: m.user.email }))];
  const teamOptions = [{ value: NONE, label: t("review.noTeam") }, ...teams.map((x) => ({ value: x.id, label: x.name }))];
  const matched = check.agents.filter((a) => agents[a.id]).length;
  return (
    <div className="flex flex-col gap-6">
      {check.agents.length > 0 && (
        <div>
          <div className="mb-2 flex items-end justify-between gap-3">
            <div>
              <h3 className="text-[13px] font-semibold tracking-[-0.01em]">{t("review.agentsTitle")}</h3>
              <p className="mt-0.5 text-[12px] leading-[18px] text-fg-3">{t("review.agentsHint")}</p>
            </div>
            <span className="shrink-0 text-[11.5px] tabular-nums text-fg-3">{t("review.matched", { n: matched, total: check.agents.length })}</span>
          </div>
          <div className="overflow-hidden rounded-lg border border-line bg-surface shadow-xs">
            {check.agents.map((a) => {
              const v = agents[a.id] ?? null;
              const auto = !!a.user_id && v === a.user_id;
              return (
                <div key={a.id} className="flex flex-col gap-2 border-b border-line-soft px-4 py-2.5 last:border-0 sm:flex-row sm:items-center">
                  <div className="flex min-w-0 flex-1 items-center gap-2.5">
                    <Avatar name={a.name || a.email} size="md" />
                    <div className="min-w-0">
                      <div className="truncate text-[12.5px] font-medium text-fg">{a.name || a.email}</div>
                      {a.email && <div className="truncate text-[11.5px] text-fg-3">{a.email}</div>}
                    </div>
                  </div>
                  <div className="flex items-center gap-2 sm:w-[300px]">
                    <span className={cn("hidden w-[86px] shrink-0 items-center gap-1 text-[11px] sm:flex", auto ? "text-tone-green" : "text-transparent")} aria-hidden={!auto}>
                      <Check className="size-3" />
                      {t("review.sameEmail")}
                    </span>
                    <Select
                      size="sm"
                      aria-label={t("review.agentFor", { name: a.name || a.email })}
                      value={v ?? NONE}
                      onChange={(x) => onAgents({ ...agents, [a.id]: x === NONE ? null : x })}
                      options={memberOptions}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
      {check.groups.length > 0 && (
        <div>
          <div className="mb-2">
            <h3 className="text-[13px] font-semibold tracking-[-0.01em]">{t("review.groupsTitle")}</h3>
            <p className="mt-0.5 text-[12px] leading-[18px] text-fg-3">{t("review.groupsHint")}</p>
          </div>
          <div className="overflow-hidden rounded-lg border border-line bg-surface shadow-xs">
            {check.groups.map((g) => (
              <div key={g.id} className="flex flex-col gap-2 border-b border-line-soft px-4 py-2.5 last:border-0 sm:flex-row sm:items-center">
                <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-fg">{g.name}</span>
                <div className="sm:w-[300px] sm:pl-[94px]">
                  <Select
                    size="sm"
                    aria-label={t("review.teamFor", { name: g.name })}
                    value={groups[g.id] ?? NONE}
                    onChange={(x) => onGroups({ ...groups, [g.id]: x === NONE ? null : x })}
                    options={teamOptions}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/** The mapping a check suggests: agents matched by email, groups by team name. */
export function suggestedMapping(check: ImportCheck) {
  return {
    agents: Object.fromEntries(check.agents.map((a) => [a.id, a.user_id])),
    groups: Object.fromEntries(check.groups.map((g) => [g.id, g.team_id])),
  };
}
