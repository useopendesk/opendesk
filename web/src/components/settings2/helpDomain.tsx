// A brand's help center custom domain (contracts/teams.md "Help center addresses"):
// connect a hostname, add the two DNS records it answers with, and watch it go
// from pending to active. Checks run on the server's schedule; every status change
// arrives as brand.updated, which lands in the brands list this reads from.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { AlertTriangle, Check, ChevronDown, CircleCheck, Clock, ExternalLink, Globe, RefreshCw, Trash2, XCircle } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Section } from "@/components/layout";
import { IconTile, MonoChip, StatePill, type ItemState } from "@/components/settings2/bits";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { CopyButton } from "@/components/ui/copy-button";
import { TextInput } from "@/components/ui/input";
import { toast } from "@/components/ui/toast";
import { Tooltip } from "@/components/ui/tooltip";
import { api } from "@/lib/api/endpoints";
import { errorMessage, isApiError } from "@/lib/api/client";
import type { Brand, HelpDomain, HelpDomainRecord } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { fullDate, relLong } from "@/lib/format";
import { bareUrl, looksLikeHostname, normalizeHostname, relativeRecordName } from "@/lib/helpCenter";
import { dur, ease } from "@/lib/motion";
import { qk, upsertInList } from "@/lib/queries";
import { tone, type Tone } from "@/lib/tones";
import { useNow } from "@/lib/useNow";

type View = { state: ItemState; label: string; hue: Tone; icon: ReactNode };

/** How a domain reads at a glance: the pill and the colour of its explanation. */
function useView(d: HelpDomain): View {
  const { t } = useTranslation("settingsTeams");
  if (d.status === "failed") return { state: "failing", label: t("domain.status.failed"), hue: "red", icon: <XCircle /> };
  if (d.status === "active" && d.problem) return { state: "pending", label: t("domain.status.attention"), hue: "amber", icon: <AlertTriangle /> };
  if (d.status === "active") return { state: "active", label: t("domain.status.active"), hue: "green", icon: <CircleCheck /> };
  if (d.problem === "certificate_pending") return { state: "pending", label: t("domain.status.securing"), hue: "amber", icon: <Clock /> };
  return { state: "pending", label: t("domain.status.pending"), hue: "amber", icon: <Clock /> };
}

/** A soft, tinted line of explanation in the domain's colour. */
function Callout({ hue, icon, children }: { hue: Tone; icon: ReactNode; children: ReactNode }) {
  return (
    <div className={cn("flex items-start gap-2 rounded-md px-3 py-2 text-[12px] leading-[18px]", tone[hue].soft, tone[hue].text)} role="status">
      <span className="mt-px flex shrink-0 [&_svg]:size-3.5" aria-hidden>
        {icon}
      </span>
      <div className="min-w-0">{children}</div>
    </div>
  );
}

export function HelpDomainSection({ brand }: { brand: Brand }) {
  const { t } = useTranslation("settingsTeams");
  const d = brand.help_center.domain;
  return (
    <Section title={t("domain.title")} description={t("domain.hint")} flush={!!d}>
      {d ? <DomainCard brand={brand} domain={d} /> : <ConnectRow brand={brand} />}
    </Section>
  );
}

function ConnectRow({ brand }: { brand: Brand }) {
  const { t } = useTranslation("settingsTeams");
  const qc = useQueryClient();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const connect = useMutation({
    mutationFn: (hostname: string) => api.brands.setHelpDomain(brand.id, hostname),
    onSuccess: (b) => upsertInList(qc, qk.brands, b),
    onError: (e) => {
      if (isApiError(e, "domain_taken")) return setError(t("domain.taken"));
      if (isApiError(e, "rate_limited")) return setError(t("domain.tooMany"));
      // hostname_invalid: the detail is a sentence about this hostname.
      setError(isApiError(e, "hostname_invalid") && e.detail ? e.detail : errorMessage(e));
    },
  });
  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const host = normalizeHostname(value);
    if (!looksLikeHostname(host)) return setError(t("domain.invalid"));
    setValue(host);
    setError(null);
    connect.mutate(host);
  };
  return (
    <div className="flex flex-col gap-3 py-3.5 md:flex-row md:items-start md:justify-between md:gap-8">
      <div className="min-w-0 md:max-w-[340px]">
        <label htmlFor="help-domain" className="text-[13px] font-medium">
          {t("domain.label")}
        </label>
        <p className="mt-0.5 text-[12px] leading-[18px] text-fg-3">{t("domain.labelHint")}</p>
      </div>
      <form className="flex min-w-0 flex-col gap-1.5" onSubmit={submit} noValidate>
        <div className="flex items-center gap-2">
          <TextInput
            id="help-domain"
            className="mono w-full md:w-[240px]"
            leading={<Globe />}
            placeholder={t("domain.placeholder")}
            value={value}
            invalid={!!error}
            autoComplete="off"
            spellCheck={false}
            inputMode="url"
            aria-describedby={error ? "help-domain-error" : connect.isPending ? "help-domain-busy" : undefined}
            disabled={connect.isPending}
            onChange={(e) => {
              setValue(e.target.value);
              if (error) setError(null);
            }}
          />
          <Button variant="primary" size="md" type="submit" loading={connect.isPending} disabled={!value.trim()}>
            {connect.isPending ? t("domain.connecting") : t("domain.connect")}
          </Button>
        </div>
        {error ? (
          <p id="help-domain-error" role="alert" className="max-w-[340px] text-[12px] leading-[18px] text-danger-fg">
            {error}
          </p>
        ) : connect.isPending ? (
          <p id="help-domain-busy" className="text-[12px] text-fg-3">
            {t("domain.connectingHint")}
          </p>
        ) : null}
      </form>
    </div>
  );
}

function DomainCard({ brand, domain: d }: { brand: Brand; domain: HelpDomain }) {
  const { t } = useTranslation("settingsTeams");
  const qc = useQueryClient();
  const confirm = useConfirm();
  const reduce = useReducedMotion();
  const now = useNow();
  const view = useView(d);
  const live = d.status === "active";
  const allFound = d.records.every((r) => r.ok);
  const [showRecords, setShowRecords] = useState(false);
  const recordsOpen = !live || !allFound || showRecords;
  const cname = d.records.find((r) => r.type === "CNAME");
  const target = cname?.value ?? "";
  const fallback = bareUrl(brand.help_center.default_url);

  const check = useMutation({
    mutationFn: () => api.brands.checkHelpDomain(brand.id),
    onSuccess: (b) => {
      upsertInList(qc, qk.brands, b);
      const next = b.help_center.domain;
      if (next?.status === "active" && d.status !== "active") toast.success(t("domain.nowActive", { hostname: d.hostname }));
      else if (next && next.status !== "active" && next.status === d.status && next.problem === d.problem) toast.info(t("domain.checkedNow"));
    },
    onError: (e) => (isApiError(e, "rate_limited") ? toast.info(t("domain.tooMany")) : toast.error(e)),
  });

  const remove = () =>
    confirm.show(
      t("domain.removeBody", { hostname: d.hostname, url: fallback }),
      async () => {
        await api.brands.removeHelpDomain(brand.id);
        upsertInList(qc, qk.brands, { ...brand, help_center: { ...brand.help_center, domain: null, url: brand.help_center.default_url } });
        toast.success(t("domain.removed", { hostname: d.hostname }));
      },
      { title: t("domain.removeTitle", { hostname: d.hostname }), confirmLabel: t("domain.remove"), danger: true },
    );

  const problem = d.problem ? t(`domain.problem.${d.problem}`, { hostname: d.hostname, target }) : "";

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 border-b border-line-soft px-4 py-3">
        <IconTile tone={view.hue === "green" ? "green" : view.hue === "red" ? "red" : "gray"}>
          <Globe />
        </IconTile>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            {live ? (
              <a
                href={`https://${d.hostname}`}
                target="_blank"
                rel="noreferrer"
                className="mono group inline-flex min-w-0 items-center gap-1 rounded-sm text-[13px] font-medium text-fg hover:underline"
                aria-label={t("domain.open", { hostname: d.hostname })}
              >
                <span className="truncate">{d.hostname}</span>
                <ExternalLink className="size-3 shrink-0 text-fg-3 group-hover:text-fg" aria-hidden />
              </a>
            ) : (
              <span className="mono truncate text-[13px] font-medium">{d.hostname}</span>
            )}
            <StatePill state={view.state} pulse={d.status === "pending"}>
              {view.label}
            </StatePill>
          </div>
          <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 text-[12px] text-fg-3">
            {d.checked_at ? (
              <Tooltip content={fullDate(d.checked_at)}>
                <span>{t("domain.checked", { when: relLong(d.checked_at, now) })}</span>
              </Tooltip>
            ) : (
              <span>{t("domain.notChecked")}</span>
            )}
            {d.status === "pending" && <span className="text-fg-4">· {t("domain.auto")}</span>}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button icon={<RefreshCw className={cn("size-3.5", check.isPending && !reduce && "animate-spin")} />} disabled={check.isPending} onClick={() => check.mutate()}>
            {t("domain.checkNow")}
          </Button>
          <Tooltip content={t("domain.remove")}>
            <Button variant="ghost" square aria-label={t("domain.remove")} onClick={remove}>
              <Trash2 className="size-3.5" />
            </Button>
          </Tooltip>
        </div>
      </div>

      <div className="flex flex-col gap-2 px-4 py-3">
        {live && !d.problem && (
          <Callout hue="green" icon={view.icon}>
            {t("domain.activeBody", { url: `https://${d.hostname}`, fallback })}
          </Callout>
        )}
        {live && d.problem && (
          <Callout hue="amber" icon={view.icon}>
            {t("domain.activeProblem")} {problem}
          </Callout>
        )}
        {d.status === "failed" && (
          <Callout hue="red" icon={view.icon}>
            {t("domain.failedBody")} {problem}
          </Callout>
        )}
        {d.status === "pending" && problem && (
          <Callout hue="amber" icon={view.icon}>
            {problem}
          </Callout>
        )}
        {!brand.help_center.enabled && (
          <Callout hue="gray" icon={<AlertTriangle />}>
            {t("domain.off")}
          </Callout>
        )}
      </div>

      {live && allFound && (
        <div className="border-t border-line-soft px-2 py-1.5">
          <Button variant="ghost" size="xs" aria-expanded={showRecords} aria-controls="help-domain-records" onClick={() => setShowRecords((v) => !v)}>
            <ChevronDown className={cn("size-3 transition-transform duration-150 motion-reduce:transition-none", showRecords && "rotate-180")} aria-hidden />
            {showRecords ? t("domain.hideRecords") : t("domain.showRecords")}
          </Button>
        </div>
      )}

      <AnimatePresence initial={false}>
        {recordsOpen && (
          <motion.div
            id="help-domain-records"
            key="records"
            initial={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
            animate={reduce ? { opacity: 1 } : { opacity: 1, height: "auto" }}
            exit={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
            transition={{ duration: dur.base, ease: ease.out }}
            className="overflow-hidden"
          >
            <Records domain={d} />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

const GRID = "md:grid md:grid-cols-[64px_minmax(0,1fr)_minmax(0,1.15fr)_96px] md:items-center md:gap-4";

function Records({ domain: d }: { domain: HelpDomain }) {
  const { t } = useTranslation("settingsTeams");
  const txt = d.records.find((r) => r.type === "TXT");
  const zone = d.hostname.split(".").slice(-2).join(".");
  return (
    <div className="border-t border-line-soft">
      <div className={cn("hidden h-8 border-b border-line-soft bg-sunken px-4", GRID)}>
        <span className="label-caps">{t("domain.col.type")}</span>
        <span className="label-caps">{t("domain.col.name")}</span>
        <span className="label-caps">{t("domain.col.value")}</span>
        <span className="label-caps">{t("domain.col.status")}</span>
      </div>
      <ul aria-label={t("domain.records")}>
        {d.records.map((r) => (
          <RecordRow key={r.type} record={r} />
        ))}
      </ul>
      <div className="flex flex-col gap-1 border-t border-line-soft bg-sunken px-4 py-2.5 text-[12px] leading-[18px] text-fg-3">
        <p>
          {t("domain.recordsHint", { zone })}{" "}
          {txt && relativeRecordName(txt.name) !== txt.name && t("domain.relativeHint", { name: relativeRecordName(txt.name) })}
        </p>
        {d.status !== "active" && <p className="text-fg-4">{t("domain.wait")}</p>}
      </div>
    </div>
  );
}

function RecordRow({ record: r }: { record: HelpDomainRecord }) {
  const { t } = useTranslation("settingsTeams");
  return (
    <li className={cn("flex flex-col gap-1.5 border-b border-line-soft px-4 py-2.5 last:border-0", GRID)}>
      <span>
        <MonoChip>{r.type}</MonoChip>
      </span>
      <Value value={r.name} label={t("domain.copyName")} caption={t("domain.col.name")} />
      <Value value={r.value} label={t("domain.copyValue")} caption={t("domain.col.value")} />
      <span className={cn("inline-flex items-center gap-1.5 text-[12px]", r.ok ? tone.green.text : "text-fg-3")}>
        <AnimatePresence mode="popLayout" initial={false}>
          <motion.span
            key={r.ok ? "ok" : "no"}
            className="inline-flex items-center gap-1.5"
            initial={{ opacity: 0, scale: 0.8 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.8 }}
            transition={{ duration: dur.fast }}
          >
            {r.ok ? <Check className="size-3.5" aria-hidden /> : <span className="mx-[3px] size-2 rounded-full border border-fg-4" aria-hidden />}
            {r.ok ? t("domain.found") : t("domain.notFound")}
          </motion.span>
        </AnimatePresence>
      </span>
    </li>
  );
}

/** A record's name or value: mono, truncated with the whole text on hover, and a copy button. */
function Value({ value, label, caption }: { value: string; label: string; caption: string }) {
  return (
    <div className="flex min-w-0 items-center gap-1">
      <span className="label-caps w-12 shrink-0 md:hidden">{caption}</span>
      <Tooltip content={value}>
        <span className="mono min-w-0 flex-1 truncate text-[12px] text-fg" tabIndex={0}>
          {value}
        </span>
      </Tooltip>
      <CopyButton value={value} label={label} />
    </div>
  );
}
