import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import type { ReactNode } from "react";
import { ErrorState } from "@/components/ErrorState";
import { KV, PageBody, PageColumn, PageHeader, Section } from "@/components/layout";
import { CheckPill } from "@/components/pills";
import { Table, TD, TH, THead, TR } from "@/components/table";
import { Time } from "@/components/Time";
import { Badge, Dot } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { idleMs } from "@/lib/api/client";
import type { AdminHealth } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { duration, number } from "@/lib/format";
import { q } from "@/lib/queries";

/** Refresh every 15 s while the page is visible and the operator is around (an idle panel lets its session lapse). */
const EVERY = 15_000;
const IDLE = 15 * 60_000;

export default function Health() {
  const health = useQuery({ ...q.health(), refetchInterval: () => (idleMs() < IDLE ? EVERY : false), refetchIntervalInBackground: false });
  const h = health.data;

  return (
    <>
      <PageHeader
        title="Health"
        subtitle={
          h ? (
            <span className="inline-flex items-center gap-1.5">
              <Dot tone={health.isError ? "danger" : "ok"} pulse={!health.isError} /> Updated <Time iso={new Date(health.dataUpdatedAt).toISOString()} />
            </span>
          ) : undefined
        }
        actions={
          <Button variant="ghost" icon={<RefreshCw className={cn(health.isFetching && "animate-spin motion-reduce:animate-none")} />} onClick={() => void health.refetch()}>
            Refresh
          </Button>
        }
      />
      <PageBody>
        {health.isPending ? (
          <PageColumn>
            <Skeleton className="h-40 rounded-lg" />
            <div className="grid gap-5 md:grid-cols-2">
              <Skeleton className="h-44 rounded-lg" />
              <Skeleton className="h-44 rounded-lg" />
            </div>
          </PageColumn>
        ) : !h ? (
          <ErrorState error={health.error} onRetry={() => void health.refetch()} />
        ) : (
          <HealthView h={h} stale={health.isError} />
        )}
      </PageBody>
    </>
  );
}

const ok = (v: boolean, yes = "OK", no = "Down") => (v ? <Badge tone="ok">{yes}</Badge> : <Badge tone="danger">{no}</Badge>);
const on = (v: boolean) => (v ? <Badge tone="green">On</Badge> : <Badge tone="gray">Off</Badge>);
const n = (v: number) => <span className="mono tabular-nums">{number(v)}</span>;
const warnN = (v: number, warnAt = 1): ReactNode => <span className={cn("mono tabular-nums", v >= warnAt && "font-medium text-danger-fg")}>{number(v)}</span>;
const text = (v: string) => (v ? <span className="mono text-[12px]">{v}</span> : <span className="text-fg-4">None</span>);

function HealthView({ h, stale }: { h: AdminHealth; stale: boolean }) {
  const order = { fail: 0, warn: 1, ok: 2 } as const;
  const checks = [...h.checks].sort((a, b) => order[a.status] - order[b.status] || a.name.localeCompare(b.name));
  const failing = checks.filter((c) => c.status === "fail").length;
  const warning = checks.filter((c) => c.status === "warn").length;
  return (
    <PageColumn>
      {stale && <p className="text-[12px] text-danger-fg">The last refresh failed; these figures may be out of date.</p>}
      <Section
        title="Checks"
        count={checks.length}
        flush
        actions={
          <span className="text-[11.5px] text-fg-3">
            {failing ? `${failing} failing` : warning ? `${warning} warning` : "All passing"}
          </span>
        }
      >
        <ul className="divide-y divide-line-soft">
          {checks.map((c) => (
            <li key={c.name} className="flex items-start gap-3 px-4 py-2.5">
              <span className="w-[72px] shrink-0 pt-px">
                <CheckPill status={c.status} />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-[12.5px] font-medium text-fg">{c.name}</span>
                {c.detail && <span className="block text-[12px] leading-[18px] text-fg-3">{c.detail}</span>}
              </span>
            </li>
          ))}
        </ul>
      </Section>

      <div className="grid items-start gap-5 md:grid-cols-2">
        <Section title="Instance">
          <KV
            items={[
              ["Version", text(h.version)],
              ["Commit", text(h.commit)],
              ["Role", text(h.role)],
              ["Started", <Time iso={h.started_at} />],
            ]}
          />
        </Section>
        <Section title="Database">
          <KV
            items={[
              ["Status", ok(h.database.ok)],
              ["Latency", <span className="mono tabular-nums">{number(h.database.latency_ms, { maximumFractionDigits: 1 })} ms</span>],
              ["Schema", <span className="flex items-center gap-2">{n(h.database.schema_version)}{h.database.dirty && <Badge tone="danger">Dirty</Badge>}</span>],
              [
                "Pool",
                <span className="mono tabular-nums">
                  {number(h.database.pool.in_use)} in use, {number(h.database.pool.idle)} idle, {number(h.database.pool.total)} of {number(h.database.pool.max)}
                </span>,
              ],
            ]}
          />
        </Section>
        <Section title="Events">
          <KV
            items={[
              ["NATS", <span className="flex items-center gap-2">{ok(h.nats.ok)}<span className="text-[11.5px] text-fg-3">{h.nats.embedded ? "Embedded" : "Cluster"}</span></span>],
              ["Unpublished", warnN(h.relay.unpublished, 100)],
              ["Oldest unpublished", <span className="mono tabular-nums">{h.relay.oldest_unpublished_s === null ? "None" : duration(h.relay.oldest_unpublished_s)}</span>],
            ]}
          />
        </Section>
        <Section title="Mail and webhooks">
          <KV
            items={[
              ["Platform mailer", text(h.mail.driver)],
              ["Inbound failed, 24 h", warnN(h.mail.inbound_failed_24h)],
              ["Inbound pending", n(h.mail.inbound_pending)],
              ["Mailboxes in error", warnN(h.mail.mailboxes_in_error)],
              ["Webhooks failing, 24 h", warnN(h.webhooks.failing_24h)],
            ]}
          />
        </Section>
      </div>

      <Section title="Jobs" flush>
        <div className="grid grid-cols-2 gap-px border-b border-line-soft bg-line-soft sm:grid-cols-3 lg:grid-cols-6">
          {(
            [
              ["Available", h.jobs.available],
              ["Running", h.jobs.running],
              ["Retrying", h.jobs.retryable],
              ["Scheduled", h.jobs.scheduled],
              ["Discarded, 24 h", h.jobs.discarded_24h],
            ] as const
          ).map(([k, v]) => (
            <div key={k} className="bg-surface px-4 py-3">
              <div className="label-caps">{k}</div>
              <div className="mono mt-1.5 text-[15px] font-medium tabular-nums">{number(v)}</div>
            </div>
          ))}
          <div className="bg-surface px-4 py-3">
            <div className="label-caps">Oldest waiting</div>
            <div className="mono mt-1.5 text-[15px] font-medium tabular-nums">{h.jobs.oldest_available_s === null ? "None" : duration(h.jobs.oldest_available_s)}</div>
          </div>
        </div>
        {h.jobs.by_queue.length > 0 && (
          <Table>
            <THead>
              <TH>Queue</TH>
              <TH align="right">Available</TH>
              <TH align="right">Running</TH>
              <TH align="right">Retrying</TH>
            </THead>
            <tbody>
              {h.jobs.by_queue.map((r) => (
                <TR key={r.queue}>
                  <TD mono>{r.queue}</TD>
                  <TD align="right" mono>
                    {number(r.available)}
                  </TD>
                  <TD align="right" mono>
                    {number(r.running)}
                  </TD>
                  <TD align="right" mono className={r.retryable ? "text-danger-fg" : undefined}>
                    {number(r.retryable)}
                  </TD>
                </TR>
              ))}
            </tbody>
          </Table>
        )}
      </Section>

      <Section title="Configuration">
        <KV
          className="md:grid-cols-[minmax(110px,max-content)_1fr_minmax(110px,max-content)_1fr]"
          items={[
            ["Billing", on(h.config.billing)],
            ["Sign-up", on(h.config.signup_enabled)],
            ["AI provider", text(h.config.ai_provider)],
            ["Decisions", text(h.config.decisions_provider)],
            ["Embeddings", text(h.config.embeddings)],
            ["Storage", text(h.config.storage)],
            ["Mailbox IMAP", on(h.config.mailbox_imap)],
            ["System send", on(h.config.mailbox_system_send)],
            ["Push", text(h.config.push)],
            ["Help domain", text(h.config.help_domain)],
          ]}
        />
      </Section>
    </PageColumn>
  );
}
