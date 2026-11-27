// What a ticket points to: issues, tasks and pages in other tools, and other tickets that
// belong with it. Ollie adds links when it creates something from the ticket; realtime keeps
// the list current.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Lock, Plus, X } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useDeferredValue, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty";
import { SearchInput, TextInput } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { ProviderMark } from "@/components/settings2/integrations";
import { toast } from "@/components/ui/toast";
import { Tooltip } from "@/components/ui/tooltip";
import { api, type TicketListQuery } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/api/client";
import { appForUrl, appLogo } from "@/lib/appLogos";
import type { Ticket, TicketLink, TicketRelation } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { ease } from "@/lib/motion";
import { q, qk, removeFromList, upsertInList } from "@/lib/queries";
import { Badge } from "@/components/ui/badge";
import { ChannelIcon, StatusPill } from "@/components/indicators";
import { StatusIcon } from "../bits";
import { useDisplay } from "../fields";
import { requesterLabel } from "../requester";
import { SetByOllie, useAssistantName } from "../ollie";

const stateTone = { backlog: "gray", todo: "gray", in_progress: "blue", done: "green", canceled: "gray" } as const;
const stateTranslation = {
  backlog: "links.states.backlog",
  todo: "links.states.todo",
  in_progress: "links.states.in_progress",
  done: "links.states.done",
  canceled: "links.states.canceled",
} as const;

type AddKind = "link" | "ticket";

function href(l: TicketLink): string | null {
  try {
    const u = new URL(l.url);
    return u.protocol === "https:" ? u.href : null;
  } catch {
    return null;
  }
}

export function TicketLinks({ ticketId, canEdit }: { ticketId: string; canEdit: boolean }) {
  const { t } = useTranslation("inbox");
  const qc = useQueryClient();
  const reduce = useReducedMotion();
  const ollie = useAssistantName();
  const display = useDisplay();
  const [addOpen, setAddOpen] = useState(false);
  const [kind, setKind] = useState<AddKind>("link");
  const [url, setUrl] = useState("");
  const [title, setTitle] = useState("");
  const [term, setTerm] = useState("");
  const deferred = useDeferredValue(term.trim());
  const [target, setTarget] = useState<Ticket | null>(null);
  const [addError, setAddError] = useState<string | null>(null);
  const { data: links } = useQuery(q.ticketLinks(ticketId));
  const { data: relations } = useQuery(q.ticketRelations(ticketId));
  const contactId = qc.getQueryData<Ticket>(qk.ticket(ticketId))?.contact?.id;
  const results = useQuery({
    queryKey: qk.of("link-ticket-search", ticketId, deferred),
    // Without a search: the same customer's other tickets, closed ones included, else the recent ones.
    queryFn: async () => {
      if (deferred) return api.tickets.list({ q: deferred, limit: 10, merged: false });
      const recent: TicketListQuery = { limit: 10, merged: false, status: ["open", "pending", "on_hold", "solved", "closed"] };
      const theirs = contactId ? await api.tickets.list({ ...recent, contact_id: contactId }) : null;
      return theirs?.data.some((x) => x.id !== ticketId) ? theirs : api.tickets.list(recent);
    },
    enabled: addOpen && kind === "ticket",
  });
  const taken = new Set((relations ?? []).flatMap((r) => (r.related ? [r.related.id] : [])));
  const candidates = (results.data?.data ?? []).filter((x) => x.id !== ticketId && !taken.has(x.id));
  const reset = () => {
    setKind("link");
    setUrl("");
    setTitle("");
    setTerm("");
    setTarget(null);
    setAddError(null);
  };
  const { data: integrations } = useQuery({ ...q.integrations(), enabled: !!links?.length });

  const add = useMutation({
    mutationFn: (input: { url: string; title?: string }) => api.tickets.addLink(ticketId, input),
    onSuccess: (link) => {
      qc.setQueryData<TicketLink[]>(qk.ticketLinks(ticketId), (current) => {
        if (!current) return [link];
        const index = current.findIndex((item) => item.id === link.id);
        if (index < 0) return [...current, link];
        const next = current.slice();
        next[index] = link;
        return next;
      });
      setAddOpen(false);
      reset();
      toast.success(t("links.added"));
    },
    onError: (error) => setAddError(errorMessage(error)),
  });

  const addTicket = useMutation({
    mutationFn: (other: Ticket) => api.tickets.addRelated(ticketId, other.id),
    onSuccess: (relation) => {
      upsertInList(qc, qk.ticketRelations(ticketId), relation);
      setAddOpen(false);
      reset();
      toast.success(t("links.ticketAdded"));
    },
    onError: (error) => setAddError(errorMessage(error)),
  });

  const removeTicket = useMutation({
    mutationFn: (r: TicketRelation) => api.tickets.removeRelated(ticketId, r.id),
    onMutate: (r) => removeFromList(qc, qk.ticketRelations(ticketId), r.id),
    onSuccess: (_r, r) =>
      toast.undo(t("links.removed"), () => {
        if (r.related) void api.tickets.addRelated(ticketId, r.related.id).then((row) => upsertInList(qc, qk.ticketRelations(ticketId), row));
      }),
    onError: (e, r) => {
      upsertInList(qc, qk.ticketRelations(ticketId), r);
      toast.error(e);
    },
  });

  const remove = useMutation({
    mutationFn: (l: TicketLink) => api.tickets.removeLink(ticketId, l.id),
    onMutate: (l) => removeFromList(qc, qk.ticketLinks(ticketId), l.id),
    onSuccess: (_r, l) =>
      toast.undo(t("links.removed"), () => {
        void api.tickets
          .addLink(ticketId, { url: l.url, title: l.title, integration_id: l.integration_id ?? undefined })
          .then((row) => upsertInList(qc, qk.ticketLinks(ticketId), row));
      }),
    onError: (e, l) => {
      upsertInList(qc, qk.ticketLinks(ticketId), l);
      toast.error(e);
    },
  });

  if (!links?.length && !relations?.length && !canEdit) return null;
  const integrationOf = (l: TicketLink) => integrations?.find((x) => x.id === l.integration_id);
  const nameOf = (l: TicketLink) => integrationOf(l)?.name ?? new URL(l.url).host;
  // A plain link, or one from a custom server, shows the logo of the product it points to.
  const appOf = (l: TicketLink) => (appLogo(l.provider) ? l.provider : (appForUrl(l.url) ?? l.provider));

  return (
    <section className="border-b border-line-soft px-4 py-3.5" aria-label={t("links.title")}>
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="label-caps">{t("links.title")}</h3>
        {canEdit && (
          <Button variant="ghost" size="xs" icon={<Plus />} onClick={() => setAddOpen(true)}>
            {t("links.add")}
          </Button>
        )}
      </div>
      {(!!links?.length || !!relations?.length) && (
        <ul className="flex flex-col gap-0.5">
          <AnimatePresence initial={false}>
            {relations?.map((r) => (
              <motion.li
                key={r.id}
                layout={reduce ? false : "position"}
                initial={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
                animate={reduce ? { opacity: 1 } : { opacity: 1, height: "auto" }}
                exit={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
                transition={{ duration: 0.2, ease: ease.out }}
                className="group -mx-1.5 flex items-center gap-2 rounded-md px-1.5 py-1 transition-colors hover:bg-sunken"
              >
                <span aria-hidden className="flex size-5 shrink-0 select-none items-center justify-center rounded-md border border-line bg-surface text-fg-2 [&_svg]:size-3">
                  {r.related ? <ChannelIcon channel={r.related.channel} plain /> : <Lock />}
                </span>
                {r.related ? (
                  <Link to="/inbox/$folder/{-$ticketId}" params={{ folder: "open", ticketId: r.related.id }} className="min-w-0 flex-1 outline-none focus-visible:shadow-[0_0_0_2px_var(--focus)]">
                    <span className="flex min-w-0 items-center gap-1.5">
                      <span className="min-w-0 truncate text-[12.5px] text-fg-2 group-hover:text-fg" data-ph-mask>
                        {r.related.subject}
                      </span>
                      <StatusPill status={r.related.status} label={display.status(r.related.status)} hue={display.statusTone(r.related.status)} />
                    </span>
                    <span className="block truncate text-[11px] text-fg-4">#{r.related.number}</span>
                  </Link>
                ) : (
                  <span className="min-w-0 flex-1 truncate text-[12.5px] text-fg-3">{t("links.privateTicket")}</span>
                )}
                {canEdit && (
                  <Tooltip content={t("links.remove")}>
                    <button
                      type="button"
                      aria-label={t("links.removeNamed", { title: r.related ? `#${r.related.number}` : t("links.privateTicket") })}
                      onClick={() => removeTicket.mutate(r)}
                      className="flex size-5 shrink-0 items-center justify-center rounded-sm text-fg-4 opacity-0 transition-opacity hover:bg-hover hover:text-fg focus-visible:opacity-100 group-hover:opacity-100"
                    >
                      <X className="size-3" />
                    </button>
                  </Tooltip>
                )}
              </motion.li>
            ))}
            {links?.map((l) => {
              const url = href(l);
              const source = nameOf(l);
              const state = l.external_state;
              const stateLabel = state && (l.external_state_name || t(stateTranslation[state]));
              return (
                <motion.li
                  key={l.id}
                  layout={reduce ? false : "position"}
                  initial={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
                  animate={reduce ? { opacity: 1 } : { opacity: 1, height: "auto" }}
                  exit={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
                  transition={{ duration: 0.2, ease: ease.out }}
                  className="group -mx-1.5 flex items-center gap-2 rounded-md px-1.5 py-1 transition-colors hover:bg-sunken"
                >
                  <span className="relative">
                    <ProviderMark provider={appOf(l)} name={source} icon={integrationOf(l)?.icon_url} size="sm" />
                    {l.actor_type === "ai" && <SetByOllie size={10} note={t("links.byOllie", { name: ollie })} className="absolute -right-1 -top-1 ring-1 ring-surface" />}
                  </span>
                  {url ? (
                    <a href={url} target="_blank" rel="noopener noreferrer" className="min-w-0 flex-1 outline-none" title={l.url}>
                      <span className="flex min-w-0 items-center gap-1.5">
                        <span className="min-w-0 truncate text-[12.5px] text-fg-2 group-hover:text-fg">{l.title || source}</span>
                        {state && stateLabel && <Badge tone={stateTone[state]}>{stateLabel}</Badge>}
                      </span>
                      <span className="block truncate text-[11px] text-fg-4">{source}</span>
                    </a>
                  ) : (
                    <span className="min-w-0 flex-1 truncate text-[12.5px] text-fg-3">{l.title}</span>
                  )}
                  {canEdit && (
                    <Tooltip content={t("links.remove")}>
                      <button
                        type="button"
                        aria-label={t("links.removeNamed", { title: l.title || source })}
                        onClick={() => remove.mutate(l)}
                        className="flex size-5 shrink-0 items-center justify-center rounded-sm text-fg-4 opacity-0 transition-opacity hover:bg-hover hover:text-fg focus-visible:opacity-100 group-hover:opacity-100"
                      >
                        <X className="size-3" />
                      </button>
                    </Tooltip>
                  )}
                </motion.li>
              );
            })}
          </AnimatePresence>
        </ul>
      )}
      <Dialog
        open={addOpen}
        onOpenChange={(open) => {
          setAddOpen(open);
          if (!open) reset();
        }}
        title={t("links.addTitle")}
        description={kind === "ticket" ? t("links.addTicketDescription") : t("links.addDescription")}
        footer={
          <>
            <Button variant="ghost" onClick={() => setAddOpen(false)}>
              {t("common.cancel")}
            </Button>
            {kind === "ticket" ? (
              <Button variant="primary" disabled={!target} loading={addTicket.isPending} onClick={() => target && addTicket.mutate(target)}>
                {t("links.add")}
              </Button>
            ) : (
              <Button variant="primary" type="submit" form="add-ticket-link" loading={add.isPending}>
                {t("links.add")}
              </Button>
            )}
          </>
        }
      >
        <Segmented
          className="mb-4"
          aria-label={t("links.kindLabel")}
          value={kind}
          onChange={(v) => {
            setKind(v);
            setAddError(null);
          }}
          options={[
            { value: "link", label: t("links.kind.link") },
            { value: "ticket", label: t("links.kind.ticket") },
          ]}
        />
        {kind === "link" ? (
          <form
            id="add-ticket-link"
            className="flex flex-col gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault();
              setAddError(null);
              let parsedURL: URL;
              try {
                parsedURL = new URL(url.trim());
              } catch {
                setAddError(t("links.httpsRequired"));
                return;
              }
              if (parsedURL.protocol !== "https:") {
                setAddError(t("links.httpsRequired"));
                return;
              }
              add.mutate({ url: parsedURL.href, title: title.trim() || undefined });
            }}
          >
            <label className="flex flex-col gap-1.5 text-[12.5px] font-medium text-fg-2">
              {t("links.url")}
              <TextInput
                type="url"
                inputSize="lg"
                value={url}
                onChange={(event) => setUrl(event.currentTarget.value)}
                placeholder="https://linear.app/acme/issue/ENG-123"
                required
                autoFocus
              />
            </label>
            <label className="flex flex-col gap-1.5 text-[12.5px] font-medium text-fg-2">
              {t("links.linkTitle")}
              <TextInput
                inputSize="lg"
                value={title}
                onChange={(event) => setTitle(event.currentTarget.value)}
                maxLength={300}
              />
            </label>
            {addError && <p role="alert" className="text-xs text-danger-fg">{addError}</p>}
          </form>
        ) : (
          <div className="flex flex-col gap-3">
            <SearchInput value={term} onChange={setTerm} placeholder={t("links.searchTickets")} inputSize="md" autoFocus />
            <div className="flex min-h-[220px] flex-col gap-0.5">
              {results.isPending ? (
                <div className="flex flex-col gap-0.5" aria-busy="true">
                  {[0, 1, 2, 3].map((i) => (
                    <div key={i} className="flex h-10 items-center gap-2.5 px-2.5" style={{ opacity: 1 - i * 0.2 }}>
                      <Skeleton className="size-3.5 rounded-full" />
                      <Skeleton className="h-2.5 w-10" />
                      <Skeleton className="h-2.5 flex-1" />
                    </div>
                  ))}
                </div>
              ) : !candidates.length ? (
                <EmptyState compact lit={2} title={t("links.noTickets")} description={t("links.noTicketsHint")} />
              ) : (
                candidates.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => setTarget(c)}
                    aria-pressed={target?.id === c.id}
                    className={cn(
                      "flex h-10 items-center gap-2.5 rounded-md px-2.5 text-left text-[12.5px] outline-none transition-[background-color,box-shadow] duration-150 hover:bg-hover focus-visible:shadow-[0_0_0_2px_var(--focus)] active:scale-[0.995]",
                      target?.id === c.id && "bg-active shadow-[inset_0_0_0_1px_var(--line-strong)]",
                    )}
                  >
                    <StatusIcon status={c.status} />
                    <span className="mono w-12 shrink-0 text-fg-3">#{c.number}</span>
                    <span className="min-w-0 flex-1 truncate" data-ph-mask>
                      {c.subject}
                    </span>
                    <span className="shrink-0 truncate text-xs text-fg-3" data-ph-mask>
                      {requesterLabel(c.contact)}
                    </span>
                  </button>
                ))
              )}
            </div>
            {addError && <p role="alert" className="text-xs text-danger-fg">{addError}</p>}
          </div>
        )}
      </Dialog>
    </section>
  );
}
