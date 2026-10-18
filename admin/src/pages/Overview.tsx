import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { BarChart } from "@/components/BarChart";
import { ErrorState } from "@/components/ErrorState";
import { PageBody, PageColumn, PageHeader, Section, StatTile } from "@/components/layout";
import { CheckPill } from "@/components/pills";
import { Skeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import type { AdminHealth, AdminSummary } from "@/lib/api/types";
import { dayLabel, number, percent, usd } from "@/lib/format";
import { q } from "@/lib/queries";
import type { Tone } from "@/lib/tones";

type Series = "signups" | "workspaces" | "ai";
const SERIES: Record<Series, { label: string; hue: Tone; value: (d: AdminSummary["daily"][number]) => number; format: (v: number) => string }> = {
  signups: { label: "Signups", hue: "blue", value: (d) => d.signups, format: (v) => number(v) },
  workspaces: { label: "New workspaces", hue: "teal", value: (d) => d.workspaces, format: (v) => number(v) },
  ai: { label: "AI charged", hue: "violet", value: (d) => d.ai_charged_usd, format: (v) => usd(v) },
};

export default function Overview() {
  const summary = useQuery(q.summary());
  const health = useQuery(q.health());
  const [series, setSeries] = useState<Series>("signups");

  return (
    <>
      <PageHeader title="Overview" subtitle="This instance at a glance" />
      <PageBody>
        <PageColumn>
          <HealthStrip health={health.data} loading={health.isPending} error={health.error} />
          {summary.isPending ? (
            <TilesSkeleton />
          ) : summary.isError ? (
            <ErrorState error={summary.error} onRetry={() => void summary.refetch()} />
          ) : (
            <>
              <Tiles s={summary.data} />
              <Section
                title="Last 30 days"
                actions={
                  <Segmented
                    size="xs"
                    value={series}
                    onChange={setSeries}
                    options={(Object.keys(SERIES) as Series[]).map((k) => ({ value: k, label: SERIES[k].label }))}
                  />
                }
              >
                <BarChart
                  label={SERIES[series].label}
                  hue={SERIES[series].hue}
                  format={SERIES[series].format}
                  data={summary.data.daily.map((d) => ({ key: d.date, label: dayLabel(d.date), value: SERIES[series].value(d) }))}
                />
              </Section>
            </>
          )}
        </PageColumn>
      </PageBody>
    </>
  );
}

function Tiles({ s }: { s: AdminSummary }) {
  const margin = s.ai.month_charged_usd - s.ai.month_provider_cost_usd;
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile label="Users" value={number(s.users.total)} sub={`+${number(s.users.new_7d)} this week, +${number(s.users.new_30d)} in 30 days`} />
        <StatTile label="Active users, 7 days" value={number(s.users.active_7d)} sub={s.users.total ? `${percent(s.users.active_7d / s.users.total)} of all users` : "Seen in the app"} />
        <StatTile label="Workspaces" value={number(s.workspaces.total)} sub={`+${number(s.workspaces.new_7d)} this week, +${number(s.workspaces.new_30d)} in 30 days`} />
        <StatTile
          label="Active workspaces, 7 days"
          value={number(s.workspaces.active_7d)}
          sub={s.workspaces.suspended ? `${number(s.workspaces.suspended)} suspended` : "None suspended"}
        />
      </div>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {s.billing.enabled && (
          <>
            <StatTile label="Paid workspaces" value={number(s.billing.paid_workspaces)} sub={s.billing.past_due ? `${number(s.billing.past_due)} past due` : "None past due"} />
            <StatTile label="Billed seats" value={number(s.billing.billed_seats)} />
          </>
        )}
        <StatTile label="AI charged this month" value={usd(s.ai.month_charged_usd)} sub={`${number(s.ai.month_calls)} calls`} />
        <StatTile
          label="AI provider cost this month"
          value={usd(s.ai.month_provider_cost_usd)}
          sub={s.billing.enabled ? `${usd(margin)} ${margin >= 0 ? "above" : "below"} cost` : "What the provider billed this instance"}
        />
      </div>
    </div>
  );
}

function TilesSkeleton() {
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4" aria-busy="true" aria-label="Loading">
      {Array.from({ length: 8 }).map((_, i) => (
        <div key={i} className="rounded-lg border border-line px-4 py-3.5">
          <Skeleton className="h-2 w-20" />
          <Skeleton className="mt-3 h-5 w-16" />
          <Skeleton className="mt-2.5 h-2 w-28" />
        </div>
      ))}
    </div>
  );
}

function HealthStrip({ health, loading, error }: { health: AdminHealth | undefined; loading: boolean; error: unknown }) {
  const checks = health?.checks ?? [];
  const failing = checks.filter((c) => c.status !== "ok");
  return (
    <Link
      to="/health"
      className="group flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border border-line bg-surface px-4 py-2 transition-colors hover:border-line-strong hover:bg-sunken"
    >
      <span className="text-[12.5px] font-medium text-fg">Health</span>
      {loading ? (
        <Skeleton className="h-3 w-48" />
      ) : error ? (
        <span className="text-[12px] text-danger-fg">Couldn't load the health checks</span>
      ) : failing.length === 0 ? (
        <span className="flex items-center gap-2 text-[12px] text-fg-3">
          <CheckPill status="ok" /> All {checks.length} checks pass
        </span>
      ) : (
        failing.slice(0, 5).map((c) => (
          <span key={c.name} className="flex items-center gap-1.5 text-[12px] text-fg-2">
            <CheckPill status={c.status} />
            {c.name}
          </span>
        ))
      )}
      <ArrowRight className="ml-auto size-3.5 text-fg-4 transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none" />
    </Link>
  );
}
