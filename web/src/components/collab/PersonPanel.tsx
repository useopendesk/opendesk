// Who a conversation is with (contracts/collab.md "People"): each outside person on it,
// your other conversations with them and the files they sent, from the mail you see.
// Docked beside the conversation on wide screens, a sheet over it on narrower ones.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Info, Mails, PenSquare } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { MailAddress, MailPersonFile, MailThread, MailThreadDetail } from "@/lib/api/types";
import { compose } from "@/lib/collab/compose";
import { useMailAccounts } from "@/lib/collab/hooks";
import { formatAddress } from "@/lib/collab/recipients";
import { firstName, peopleOf, personName } from "@/lib/collab/people";
import { cn } from "@/lib/cn";
import { bytes, number, relShort, shortDate } from "@/lib/format";
import { q } from "@/lib/queries";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip } from "@/components/ui/tooltip";
import { AccountDot } from "./bits";

export type PersonPanelProps = {
  detail: MailThreadDetail;
  /** Opens another conversation in place of this one. */
  onOpenThread: (id: string) => void;
  /** Lists every conversation with the person. */
  onShowAll: (email: string) => void;
};

export function PersonPanel({ detail, onOpenThread, onShowAll }: PersonPanelProps) {
  const { t } = useTranslation("collab");
  const { data: accounts } = useMailAccounts();
  const people = useMemo(() => peopleOf(detail.thread.participants, [detail.account, ...(accounts ?? [])]), [detail, accounts]);
  const [picked, setPicked] = useState<string | null>(null);
  const current = people.find((p) => p.email === picked) ?? people[0];

  if (!current)
    return (
      <div className="flex flex-col gap-1.5 p-4">
        <span className="label-caps">{t("people.title")}</span>
        <p className="text-xs leading-[18px] text-fg-3">{t("people.onlyYou")}</p>
      </div>
    );

  return (
    <div className="flex flex-col">
      {people.length > 1 && (
        <div className="flex flex-col gap-2 border-b border-line-soft px-4 pb-3 pt-3.5">
          <span className="label-caps">{t("people.onConversation", { count: people.length })}</span>
          <div role="tablist" aria-label={t("people.onConversation", { count: people.length })} className="flex flex-wrap gap-1.5">
            {people.map((p) => {
              const on = p.email === current.email;
              return (
                <Tooltip key={p.email} content={personName(p)}>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={on}
                    aria-label={personName(p)}
                    onClick={() => setPicked(p.email)}
                    className={cn(
                      "rounded-md outline-none ring-offset-2 ring-offset-surface transition-[box-shadow,opacity] duration-150 focus-visible:shadow-[0_0_0_2px_var(--focus)] motion-reduce:transition-none",
                      on ? "ring-2 ring-fg" : "opacity-60 hover:opacity-100",
                    )}
                  >
                    <Avatar name={personName(p)} size="md" />
                  </button>
                </Tooltip>
              );
            })}
          </div>
        </div>
      )}
      <PersonCard key={current.email} who={current} detail={detail} onOpenThread={onOpenThread} onShowAll={onShowAll} />
    </div>
  );
}

function PersonCard({ who, detail, onOpenThread, onShowAll }: { who: MailAddress } & PersonPanelProps) {
  const { t } = useTranslation("collab");
  const qc = useQueryClient();
  const { data: accounts } = useMailAccounts();
  const { data: p, isPending, isError, refetch } = useQuery(q.mailPerson(who.email, detail.thread.id));
  const shown = { name: p?.name || who.name, email: who.email };
  const name = personName(shown);
  const first = firstName(shown);
  // Account dots only say something when there is more than one account.
  const manyAccounts = (accounts?.length ?? 0) > 1;
  // New mail goes from an account you see; this conversation's when you see it.
  const canWrite = detail.can_act && !!accounts?.length;
  const writeFrom = accounts?.some((a) => a.id === detail.account.id) ? detail.account.id : undefined;

  return (
    <div className="flex flex-col gap-4 px-4 pb-5 pt-4">
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <Avatar name={name} size="xl" />
          <div className="min-w-0 flex-1">
            <div className="truncate text-[14px] font-semibold tracking-[-0.01em]" data-ph-mask>
              {name}
            </div>
            <div className="group flex min-w-0 items-center gap-1">
              <span className="mono min-w-0 truncate text-[11.5px] text-fg-3" data-ph-mask>
                {who.email}
              </span>
              <CopyButton value={who.email} label={t("people.copy")} className="size-5 shrink-0 opacity-0 group-hover:opacity-100 focus-visible:opacity-100" />
            </div>
          </div>
        </div>
        <div className="flex gap-1.5">
          {canWrite && (
            <Button size="sm" variant="secondary" icon={<PenSquare />} className="flex-1" onClick={() => compose.open({ accountId: writeFrom, to: [formatAddress({ name: shown.name, email: who.email })] })}>
              {t("people.write")}
            </Button>
          )}
          <Button size="sm" variant="secondary" icon={<Mails />} className="flex-1" onClick={() => onShowAll(who.email)}>
            {t("people.allMail")}
          </Button>
        </div>
      </div>

      {isPending ? (
        <PersonSkeleton />
      ) : isError || !p ? (
        <div className="flex flex-col items-start gap-2 rounded-md bg-sunken px-3 py-2.5 text-xs text-fg-3">
          {t("people.error")}
          <Button size="xs" variant="secondary" onClick={() => void refetch()}>
            {t("list.error.retry")}
          </Button>
        </div>
      ) : (
        <>
          <dl className="grid grid-cols-2 gap-px overflow-hidden rounded-md border border-line bg-line">
            <Stat label={t("people.conversations")} value={number(p.conversations)} />
            <Stat label={t("people.since")} value={p.first_at ? shortDate(p.first_at) : "–"} hint={t("people.sinceHint")} />
          </dl>

          <section className="flex flex-col gap-1">
            <div className="flex items-center justify-between">
              <h3 className="label-caps">{t("people.other")}</h3>
              {p.others > 0 && <span className="mono text-[11px] tabular-nums text-fg-4">{number(p.others)}</span>}
            </div>
            {p.threads.length === 0 ? (
              <p className="py-1 text-xs leading-[18px] text-fg-4">{t("people.noOther", { name: first })}</p>
            ) : (
              <ul className="-mx-2 flex flex-col">
                {p.threads.map((x) => (
                  <ThreadLine
                    key={x.id}
                    x={x}
                    account={manyAccounts ? accounts?.find((a) => a.id === x.account_id) : undefined}
                    onOpen={() => onOpenThread(x.id)}
                    onIntent={() => void qc.prefetchQuery(q.mailThread(x.id))}
                  />
                ))}
              </ul>
            )}
            {p.others > p.threads.length && (
              <button
                type="button"
                onClick={() => onShowAll(who.email)}
                className="-mx-2 mt-0.5 flex h-7 items-center rounded-md px-2 text-left text-xs text-fg-3 outline-none transition-colors hover:bg-hover hover:text-fg focus-visible:shadow-[0_0_0_2px_var(--focus)]"
              >
                {t("people.showAll", { count: p.others })}
              </button>
            )}
          </section>

          {p.files.length > 0 && (
            <section className="flex flex-col gap-1">
              <h3 className="label-caps">{t("people.files", { name: first })}</h3>
              <ul className="-mx-2 flex flex-col">
                {p.files.map((f) => (
                  <FileLine key={`${f.message_id}-${f.attachment.id}`} f={f} />
                ))}
              </ul>
            </section>
          )}

          <p className="flex gap-1.5 text-[11px] leading-4 text-fg-4">
            <Info className="mt-px size-3 shrink-0" />
            {t("people.scope")}
          </p>
        </>
      )}
    </div>
  );
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  const body = (
    <div className="flex flex-col gap-0.5 bg-surface px-2.5 py-2">
      <dt className="text-[11px] text-fg-3">{label}</dt>
      <dd className="text-[13px] font-medium tabular-nums">{value}</dd>
    </div>
  );
  return hint ? <Tooltip content={hint}>{body}</Tooltip> : body;
}

function ThreadLine({ x, account, onOpen, onIntent }: { x: MailThread; account?: { color: string; name: string; email: string }; onOpen: () => void; onIntent: () => void }) {
  const { t } = useTranslation("collab");
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        onPointerEnter={onIntent}
        onFocus={onIntent}
        className="flex w-full flex-col gap-0.5 rounded-md px-2 py-1.5 text-left outline-none transition-colors hover:bg-hover focus-visible:shadow-[0_0_0_2px_var(--focus)]"
      >
        <span className="flex w-full items-center gap-1.5">
          {x.unread && <span aria-label={t("action.unread")} className="size-1.5 shrink-0 rounded-full bg-tone-blue" />}
          <span className={cn("min-w-0 flex-1 truncate text-[12.5px]", x.unread ? "font-semibold text-fg" : "text-fg-2")} data-ph-mask>
            {x.subject || t("list.noSubject")}
          </span>
          {account && <AccountDot color={account.color} title={account.name || account.email} />}
          <span className="mono shrink-0 text-[10.5px] text-fg-4">{relShort(x.last_message_at)}</span>
        </span>
        {x.snippet && (
          <span className="block w-full truncate text-[11.5px] text-fg-4" data-ph-mask>
            {x.snippet}
          </span>
        )}
      </button>
    </li>
  );
}

function FileLine({ f }: { f: MailPersonFile }) {
  const { t } = useTranslation("collab");
  const a = f.attachment;
  const image = a.content_type.startsWith("image/");
  // Images show in place from the same signed URL (?inline=1).
  const thumb = /^https?:|^\//i.test(a.url) ? `${a.url}${a.url.includes("?") ? "&" : "?"}inline=1` : a.url;
  return (
    <li>
      <a
        href={a.url}
        download={a.filename}
        rel="noopener noreferrer"
        aria-label={t("thread.download", { name: a.filename })}
        className="flex items-center gap-2.5 rounded-md px-2 py-1.5 outline-none transition-colors hover:bg-hover focus-visible:shadow-[0_0_0_2px_var(--focus)]"
      >
        <span className="flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-sm border border-line bg-sunken">
          {image ? <img src={thumb} alt="" loading="lazy" referrerPolicy="no-referrer" className="ph-no-capture size-full object-cover" /> : <FileText className="size-3.5 text-fg-3" />}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[12.5px] text-fg-2" data-ph-mask>
            {a.filename || t("people.file")}
          </span>
          <span className="mono block text-[10.5px] text-fg-4">
            {bytes(a.size)} · {shortDate(f.sent_at)}
          </span>
        </span>
      </a>
    </li>
  );
}

function PersonSkeleton() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true">
      <Skeleton className="h-[50px] rounded-md" />
      <div className="flex flex-col gap-2.5">
        <Skeleton className="h-2.5 w-24" />
        {[0, 1, 2].map((i) => (
          <div key={i} className="flex flex-col gap-1.5">
            <Skeleton className="h-2.5" style={{ width: `${82 - i * 12}%` }} />
            <Skeleton className="h-2 opacity-60" style={{ width: `${64 - i * 8}%` }} />
          </div>
        ))}
      </div>
    </div>
  );
}
