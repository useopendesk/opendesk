// A WhatsApp ticket's reply (contracts/whatsapp.md "Window"): free-form words while the
// customer's last message is less than 24 hours old, an approved template after that.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Field, TextInput } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import type { Ticket, WhatsAppTemplate, WhatsAppTicketState } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { q, qk } from "@/lib/queries";
import { tone } from "@/lib/tones";

type TemplatePick = { id: string; variables: Record<string, string> };

/** A template's words with its variables put in, as the customer reads them. */
export function fillTemplate(body: string, vars: Record<string, string>): string {
  return body.replace(/\{\{\s*([0-9A-Za-z_]+)\s*\}\}/g, (_, k: string) => vars[k] ?? "");
}

const escape = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\n/g, "<br>");

export type WhatsAppReply = {
  /** The ticket is on WhatsApp and the box is replying. */
  active: boolean;
  state?: WhatsAppTicketState;
  /** The number is not connected: nothing can be sent. */
  disconnected: boolean;
  /** The window is closed: only a template can be sent. */
  closed: boolean;
  templates: WhatsAppTemplate[];
  loadingTemplates: boolean;
  pick: TemplatePick | null;
  setPick: (p: TemplatePick | null) => void;
  template?: WhatsAppTemplate;
  /** A template is chosen and every variable filled. */
  ready: boolean;
  /** What goes out as the reply's words when a template is chosen. */
  html: string;
  /** What the composer may send. */
  sendable: boolean;
};

export function useWhatsAppReply(ticket: Pick<Ticket, "id" | "channel">, mode: string): WhatsAppReply {
  const active = ticket.channel === "whatsapp" && mode === "reply";
  const qc = useQueryClient();
  const { data: state } = useQuery({ ...q.whatsappTicket(ticket.id), enabled: active });
  const closed = active && !!state?.connection && !state.window.open;
  const disconnected = active && !!state && !state.connection;
  const { data: templates, isLoading } = useQuery({ ...q.whatsappTemplates(ticket.id), enabled: closed });
  const [pick, setPick] = useState<TemplatePick | null>(null);
  // The window shuts while the box is open: the next look at it says so.
  const closesAt = state?.window.closes_at;
  useEffect(() => {
    if (!active || !closesAt || !state?.window.open) return;
    const wait = Date.parse(closesAt) - Date.now();
    if (wait <= 0 || wait > 2 ** 31 - 1) return;
    const id = setTimeout(() => void qc.invalidateQueries({ queryKey: qk.whatsappTicket(ticket.id), exact: true }), wait + 500);
    return () => clearTimeout(id);
  }, [active, closesAt, state?.window.open, qc, ticket.id]);
  const template = useMemo(() => templates?.find((x) => x.id === pick?.id), [templates, pick?.id]);
  const ready = !!template && !!pick && template.variables.every((v) => pick.variables[v]?.trim());
  const html = template && pick ? `<p>${escape(fillTemplate(template.body, pick.variables))}</p>` : "";
  const sendable = !active || (!!state?.connection && (!closed || ready));
  return { active, state, disconnected, closed, templates: templates ?? [], loadingTemplates: isLoading, pick, setPick, template, ready, html, sendable };
}

/** Where the reply goes and whether the window is open. */
export function WhatsAppLine({ ticket, wa }: { ticket: Pick<Ticket, "contact">; wa: WhatsAppReply }) {
  const { t } = useTranslation("inbox");
  const who = ticket.contact?.name || t("composer.theCustomer");
  const state = wa.state;
  return (
    <span className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-fg-3" data-testid="whatsapp-line">
      <span className="min-w-0 truncate">
        {t("composer.whatsappTo")} <span className="mono text-fg-2" data-ph-mask>{who}</span>
        {state?.connection && <span className="text-fg-4"> · {t("composer.whatsappVia", { sender: state.connection.sender })}</span>}
      </span>
      {wa.disconnected ? (
        <span className={cn("inline-flex h-5 items-center gap-1 rounded-sm px-1.5 text-[11px] font-medium", tone.red.chip)}>{t("composer.whatsappDisconnected")}</span>
      ) : state?.connection ? (
        <span className={cn("inline-flex h-5 items-center gap-1 rounded-sm px-1.5 text-[11px] font-medium", wa.closed ? tone.amber.chip : tone.green.chip)}>
          <Clock className="size-3" aria-hidden />
          {wa.closed ? t("composer.whatsappClosed") : t("composer.whatsappOpen", { when: closesIn(state.window.closes_at) })}
        </span>
      ) : null}
    </span>
  );
}

/** "5 h" or "40 min": how long the free-form window lasts. */
function closesIn(at: string | null): string {
  const mins = Math.max(0, Math.round((Date.parse(at ?? "") - Date.now()) / 60000));
  if (!Number.isFinite(mins)) return "";
  return mins >= 90 ? `${Math.round(mins / 60)} h` : `${mins} min`;
}

/** The window is closed: choose an approved template and fill its variables. */
export function WhatsAppTemplatePanel({ wa }: { wa: WhatsAppReply }) {
  const { t } = useTranslation("inbox");
  if (!wa.closed) return null;
  const options = wa.templates.map((x) => ({ value: x.id, label: x.name }));
  return (
    <div className="flex flex-col gap-3 px-3 py-3" data-testid="whatsapp-templates">
      <p className="text-[12.5px] leading-[18px] text-fg-2">{t("composer.whatsappClosedHint")}</p>
      {!wa.loadingTemplates && !wa.templates.length ? (
        <p className="text-[12.5px] text-fg-3">{t("composer.whatsappNoTemplates")}</p>
      ) : (
        <Field label={t("composer.whatsappTemplate")}>
          {(props) => (
            <Select
              {...props}
              value={wa.pick?.id ?? ""}
              placeholder={t("composer.whatsappPick")}
              onChange={(id) => wa.setPick({ id, variables: {} })}
              options={options}
            />
          )}
        </Field>
      )}
      {wa.template?.variables.map((v) => (
        <Field key={v} label={t("composer.whatsappVariable", { name: v })}>
          {(props) => (
            <TextInput
              {...props}
              maxLength={1024}
              value={wa.pick?.variables[v] ?? ""}
              onChange={(e) => wa.setPick({ id: wa.template!.id, variables: { ...wa.pick?.variables, [v]: e.target.value } })}
            />
          )}
        </Field>
      ))}
      {wa.ready && (
        <div className="rounded-md border border-line-soft bg-sunken px-3 py-2 text-[12.5px] leading-[18px] text-fg-2" data-testid="whatsapp-preview">
          {fillTemplate(wa.template!.body, wa.pick!.variables)}
        </div>
      )}
    </div>
  );
}
