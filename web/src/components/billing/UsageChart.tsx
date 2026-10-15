// AI usage in US dollars per day, stacked by feature. The top features take the chart series
// hues in order (lib/tones SERIES), the rest fold into a quiet "Other". Each
// feature keeps its hue in every bar; the legend, tooltip and table carry the names.

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { AIFeature } from "@/lib/api/types";
import { niceTicks, useWidth } from "@/components/reports/useSize";
import { SERIES, toneVar } from "@/lib/tones";
import { useFmt } from "./hooks";
import type { StackedDay } from "./model";

const H = 180;
const AXIS = 26;
const ML = 40;
const MR = 8;
const MT = 8;

const OTHER = "var(--line-strong)";

type Key = AIFeature | "other";

export function shadeOf(series: Key[], key: Key) {
  const i = series.indexOf(key);
  if (key === "other" || i < 0) return OTHER;
  return toneVar(SERIES[i % SERIES.length]);
}

export function UsageLegend({ series }: { series: Key[] }) {
  const { t } = useTranslation("billing");
  if (series.length < 2) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1">
      {series.map((k) => (
        <span key={k} className="flex items-center gap-1.5 text-xs text-fg-2">
          <span className="size-2 rounded-xs" style={{ background: shadeOf(series, k) }} />
          {t(`features.${k}`)}
        </span>
      ))}
    </div>
  );
}

export function UsageChart({ days, series }: { days: StackedDay[]; series: Key[] }) {
  const { t } = useTranslation("billing");
  const fmt = useFmt();
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);

  const plotW = Math.max(0, width - ML - MR);
  const ticks = niceTicks(Math.max(0.01, ...days.map((d) => d.total)));
  const top = ticks[ticks.length - 1];
  const slot = days.length ? plotW / days.length : 0;
  const barW = Math.max(2, Math.min(18, slot * 0.62));
  const x = (i: number) => ML + slot * i + (slot - barW) / 2;
  const y = (v: number) => MT + H - (v / top) * H;
  const last = days.length - 1;

  const labelEvery = Math.max(1, Math.ceil(days.length / Math.max(2, Math.floor(plotW / 72))));
  const h = hover !== null ? days[hover] : null;
  const tipX = hover !== null ? x(hover) + barW / 2 : 0;
  const flip = tipX > width - 190;

  return (
    <div ref={ref} className="relative w-full select-none" style={{ height: H + MT + AXIS }}>
      {width > 0 && (
        <svg width={width} height={H + MT + AXIS} className="block overflow-visible" aria-hidden>
          {ticks.map((v) => (
            <g key={v}>
              <line x1={ML} x2={ML + plotW} y1={y(v)} y2={y(v)} stroke={v === 0 ? "var(--line)" : "var(--line-soft)"} shapeRendering="crispEdges" />
              <text x={ML - 8} y={y(v)} dy="0.32em" textAnchor="end" className="mono" fontSize={10.5} fill="var(--fg-4)">
                {fmt.usd(v, { tiny: true })}
              </text>
            </g>
          ))}
          {days.map((d, i) => {
            let acc = 0;
            const segs = d.stacks.filter((s) => s.usd > 0);
            const dim = hover !== null && hover !== i;
            return (
              <g key={d.date} opacity={dim ? 0.45 : 1} style={{ transition: "opacity 120ms" }}>
                {segs.map((s, j) => {
                  const base = y(acc);
                  acc += s.usd;
                  const isTop = j === segs.length - 1;
                  // A 2px surface gap between stacked segments: 1px off each side.
                  const bottom = base - (j > 0 ? 1 : 0);
                  const top = y(acc) + (isTop ? 0 : 1);
                  const hgt = Math.max(0, bottom - top);
                  if (hgt <= 0) return null;
                  const r = isTop ? Math.min(2, barW / 2, hgt) : 0;
                  const x0 = x(i);
                  const x1 = x0 + barW;
                  return (
                    <path
                      key={s.key}
                      fill={shadeOf(series, s.key)}
                      d={`M${x0},${bottom}V${top + r}Q${x0},${top} ${x0 + r},${top}H${x1 - r}Q${x1},${top} ${x1},${top + r}V${bottom}Z`}
                    />
                  );
                })}
              </g>
            );
          })}
          {days.map((d, i) =>
            i % labelEvery === 0 ? (
              <text key={d.date} x={x(i) + barW / 2} y={MT + H + 17} textAnchor="middle" className="mono" fontSize={10.5} fill="var(--fg-4)">
                {fmt.day(d.date)}
              </text>
            ) : null,
          )}
        </svg>
      )}

      <div
        role="img"
        tabIndex={0}
        aria-label={t("usage.chartLabel", { days: days.length })}
        className="absolute rounded-md outline-none focus-visible:shadow-[0_0_0_2px_var(--focus)]"
        style={{ left: ML, top: MT, width: plotW, height: H }}
        onPointerMove={(e) => {
          if (!slot) return;
          const r = e.currentTarget.getBoundingClientRect();
          setHover(Math.min(last, Math.max(0, Math.floor((e.clientX - r.left) / slot))));
        }}
        onPointerLeave={() => setHover(null)}
        onBlur={() => setHover(null)}
        onFocus={() => setHover((v) => v ?? last)}
        onKeyDown={(e) => {
          if (e.key === "ArrowRight") setHover((v) => Math.min(last, (v ?? -1) + 1));
          else if (e.key === "ArrowLeft") setHover((v) => Math.max(0, (v ?? last + 1) - 1));
          else if (e.key === "Escape") setHover(null);
          else return;
          e.preventDefault();
        }}
      />

      {h && (
        <div
          role="status"
          className="pointer-events-none absolute z-10 min-w-[168px] rounded-md border border-line bg-raised px-2.5 py-2 shadow-pop"
          style={{ left: flip ? tipX - 10 : tipX + 10, top: MT, transform: flip ? "translateX(-100%)" : undefined }}
        >
          <div className="mb-1.5 flex items-baseline justify-between gap-4">
            <span className="mono text-[10.5px] text-fg-3">{fmt.day(h.date, true)}</span>
            <span className="mono text-[12.5px] font-semibold">{fmt.usd(h.total, { tiny: true })}</span>
          </div>
          {[...h.stacks].reverse().map((s) => (
            <div key={s.key} className="flex items-center gap-2 py-px">
              <span className="size-2 rounded-xs" style={{ background: shadeOf(series, s.key) }} />
              <span className="flex-1 text-xs text-fg-2">{t(`features.${s.key}`)}</span>
              <span className="mono text-xs tabular-nums text-fg">{fmt.usd(s.usd, { tiny: true })}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function UsageDayTable({ days, series }: { days: StackedDay[]; series: Key[] }) {
  const { t } = useTranslation("billing");
  const fmt = useFmt();
  return (
    <div className="max-h-[230px] overflow-y-auto">
      <table className="w-full text-[12.5px]">
        <caption className="sr-only">{t("usage.title")}</caption>
        <thead className="sticky top-0 bg-surface">
          <tr className="border-b border-line text-left">
            <th className="label-caps py-2 font-normal">{t("usage.day")}</th>
            {series.map((k) => (
              <th key={k} className="label-caps py-2 text-right font-normal">
                {t(`features.${k}`)}
              </th>
            ))}
            <th className="label-caps py-2 text-right font-normal">{t("usage.total")}</th>
          </tr>
        </thead>
        <tbody>
          {[...days].reverse().map((d) => (
            <tr key={d.date} className="border-b border-line-soft last:border-0">
              <td className="py-1.5 text-fg-2">{fmt.day(d.date, true)}</td>
              {d.stacks.map((s) => (
                <td key={s.key} className="mono py-1.5 text-right tabular-nums text-fg-2">
                  {fmt.usd(s.usd, { tiny: true })}
                </td>
              ))}
              <td className="mono py-1.5 text-right tabular-nums">{fmt.usd(d.total, { tiny: true })}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
