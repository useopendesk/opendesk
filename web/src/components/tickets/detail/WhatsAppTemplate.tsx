// WhatsApp on a ticket (contracts/whatsapp.md): the 24 hour customer service window, and the
// approved templates that may go out inside or after it.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Check, MessageCircle, Search, Send } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Ticket, WhatsAppTemplate } from "@/lib/api/types";
import { api } from "@/lib/api/endpoints";
import { cn } from "@/lib/cn";
import { minutes } from "@/lib/format";
import { q, qk } from "@/lib/queries";
import { tone } from "@/lib/tones";
import { useNow } from "@/lib/useNow";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, TextInput } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { toast } from "@/components/ui/toast";

/** Meta's customer service window: free-form replies are allowed for 24 hours after the customer's latest message. */
export const WHATSAPP_WINDOW_MS = 24 * 60 * 60 * 1000;

/** When the window closes for this ticket, or null when the customer never wrote. */
export function whatsAppWindowEndsAt(ticket: Ticket): number | null {
  if (!ticket.last_customer_message_at) return null;
  const at = Date.parse(ticket.last_customer_message_at);
  return Number.isFinite(at) ? at + WHATSAPP_WINDOW_MS : null;
}

/** Whether a free-form reply may go out right now. */
export function whatsAppWindowOpen(ticket: Ticket): boolean {
  const ends = whatsAppWindowEndsAt(ticket);
  return ends !== null && Date.now() < ends;
}

/** The window's state above the composer, with the way to a template either way. */
export function WhatsAppWindow({ ticket, onTemplates }: { ticket: Ticket; onTemplates: () => void }) {
  const { t } = useTranslation("inbox");
  const ends = whatsAppWindowEndsAt(ticket);
  const open = ends !== null && Date.now() < ends;
  // Only the countdown needs the clock; a closed window stands still.
  const now = useNow(open);
  const left = ends === null ? 0 : Math.max(0, ends - now);
  return (
    <div className={cn("mb-2 flex items-center gap-2 rounded-md px-3 py-2 text-[12.5px]", open ? "bg-sunken" : tone.amber.soft)}>
      <MessageCircle className={cn("size-3.5 shrink-0", open ? tone.green.text : tone.amber.text)} />
      <span className="min-w-0 flex-1 text-fg-2">
        {open ? t("composer.whatsappWindowOpen", { left: minutes(Math.ceil(left / 60_000)) }) : t("composer.whatsappWindowClosed")}
      </span>
      <Button size="xs" variant="secondary" icon={<Send className={cn("size-3", tone.green.text)} />} onClick={onTemplates}>
        {t("composer.whatsappTemplate")}
      </Button>
    </div>
  );
}

const placeholderPattern = () => /\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g;

/** The distinct placeholders of a template body, in the order they appear (what Meta expects). */
function orderedPlaceholders(body: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const m of body.matchAll(placeholderPattern())) {
    if (!m[1] || seen.has(m[1])) continue;
    seen.add(m[1]);
    out.push(m[1]);
  }
  return out;
}

/**
 * Whether this picker can send the template: a text header or none, no header variable and no
 * URL button variable. Image headers, header values and button values are sent from Meta for now.
 */
function sendable(x: WhatsAppTemplate): boolean {
  const header = (x.header_type ?? "").trim().toUpperCase();
  if (header && header !== "TEXT") return false;
  if (header && orderedPlaceholders(x.header_content ?? "").length) return false;
  return !(x.buttons ?? []).some((b) => (b.type ?? "").toUpperCase() === "URL" && /\{\{/.test(b.url ?? ""));
}

/** The body with the agent's values filled in, for the preview. */
function filled(body: string, params: Record<string, string>): string {
  return body.replace(placeholderPattern(), (match, key: string) => params[key]?.trim() || match);
}

/** Pick an approved template, fill its placeholders and send it, in the window or outside it. */
export function WhatsAppTemplateDialog({ ticket, connectionId, onClose }: { ticket: Ticket; connectionId?: string; onClose: () => void }) {
  const { t } = useTranslation("inbox");
  const qc = useQueryClient();
  const { data, isLoading } = useQuery(q.whatsappTemplates(connectionId));
  const [term, setTerm] = useState("");
  const [picked, setPicked] = useState<WhatsAppTemplate | null>(null);
  const [params, setParams] = useState<Record<string, string>>({});
  const approved = useMemo(() => (data ?? []).filter((x) => x.status.toUpperCase() === "APPROVED" && sendable(x)), [data]);
  const matches = useMemo(() => {
    const q = term.trim().toLowerCase();
    if (!q) return approved;
    return approved.filter((x) => x.name.toLowerCase().includes(q) || x.language.toLowerCase().includes(q) || x.category.toLowerCase().includes(q));
  }, [approved, term]);
  const placeholders = useMemo(() => orderedPlaceholders(picked?.body_content ?? ""), [picked]);
  const ready = !!picked && placeholders.every((p) => !!params[p]?.trim());

  const send = useMutation({
    mutationFn: () => {
      if (!picked) throw new Error("no template picked");
      const values = Object.fromEntries(placeholders.map((p) => [p, params[p]?.trim() ?? ""]));
      return api.whatsapp.sendTemplate(ticket.id, { template_id: picked.id, params: placeholders.length ? values : undefined });
    },
    onSuccess: () => {
      toast.success(t("whatsappTemplate.sent"));
      void qc.invalidateQueries({ queryKey: qk.messages(ticket.id), exact: true });
      void qc.invalidateQueries({ queryKey: qk.ticket(ticket.id) });
      onClose();
    },
    onError: (e) => toast.error(e, t("whatsappTemplate.error")),
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("whatsappTemplate.title")}
      description={t("whatsappTemplate.description")}
      footer={
        picked ? (
          <>
            <Button icon={<ArrowLeft />} onClick={() => setPicked(null)}>
              {t("whatsappTemplate.back")}
            </Button>
            <span className="flex-1" />
            <Button onClick={onClose}>{t("whatsappTemplate.cancel")}</Button>
            <Button variant="primary" icon={<Check />} disabled={!ready} loading={send.isPending} onClick={() => send.mutate()}>
              {t("whatsappTemplate.send")}
            </Button>
          </>
        ) : (
          <Button onClick={onClose}>{t("whatsappTemplate.cancel")}</Button>
        )
      }
    >
      {picked ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[13px] font-medium">{picked.name}</span>
            <span className="mono text-[11.5px] text-fg-3">{picked.language}</span>
            {picked.category && <span className="text-[11.5px] text-fg-4">{picked.category}</span>}
          </div>
          <div>
            <p className="mb-1.5 text-[11.5px] font-medium text-fg-3">{t("whatsappTemplate.preview")}</p>
            {picked.header_type?.toUpperCase() === "TEXT" && picked.header_content && (
              <p className="mb-1.5 text-[12.5px] font-medium text-fg-2">{picked.header_content}</p>
            )}
            <pre className="ph-no-capture max-h-[220px] overflow-auto whitespace-pre-wrap rounded-md border border-line bg-sunken px-3 py-2.5 text-[12.5px] leading-[19px] text-fg-2">
              {filled(picked.body_content, params)}
            </pre>
            {picked.footer_content && <p className="mt-1.5 text-[11.5px] text-fg-4">{picked.footer_content}</p>}
          </div>
          {placeholders.length > 0 && (
            <div className="flex flex-col gap-3">
              <p className="text-[11.5px] font-medium text-fg-3">{t("whatsappTemplate.params")}</p>
              {placeholders.map((p) => (
                <Field key={p} label={<span className="mono">{`{{${p}}}`}</span>} hint={t("whatsappTemplate.paramHint")}>
                  {(props) => (
                    <TextInput
                      {...props}
                      autoComplete="off"
                      value={params[p] ?? ""}
                      onChange={(e) => setParams((old) => ({ ...old, [p]: e.target.value }))}
                    />
                  )}
                </Field>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-fg-4" />
            <TextInput
              aria-label={t("whatsappTemplate.search")}
              placeholder={t("whatsappTemplate.search")}
              value={term}
              onChange={(e) => setTerm(e.target.value)}
              className="pl-8"
            />
          </div>
          {isLoading ? (
            <div className="flex items-center gap-2 py-6 text-[12.5px] text-fg-3">
              <Spinner className="size-3.5" /> {t("whatsappTemplate.loading")}
            </div>
          ) : !matches.length ? (
            <p className="py-6 text-center text-[12.5px] text-fg-3">{term.trim() ? t("whatsappTemplate.emptySearch") : t("whatsappTemplate.noTemplates")}</p>
          ) : (
            <div className="max-h-[360px] overflow-y-auto rounded-md border border-line">
              {matches.map((x) => (
                <button
                  key={x.id}
                  type="button"
                  onClick={() => setPicked(x)}
                  className="flex w-full items-center gap-2.5 border-b border-line-soft px-3 py-2.5 text-left last:border-0 hover:bg-hover"
                >
                  <span className="min-w-0 flex-1 truncate text-[13px]">{x.name}</span>
                  <span className="mono shrink-0 text-[11.5px] text-fg-3">{x.language}</span>
                  {x.category && <span className="shrink-0 text-[11.5px] text-fg-4">{x.category}</span>}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </Dialog>
  );
}
