// Small pieces shared by the AI agent pages.

import { Link } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { OllieMark } from "@/components/assistant/OllieMark";
import { cn } from "@/lib/cn";
import { formatUsd, number } from "@/lib/format";
import { AI_TONE, tone } from "@/lib/tones";

/** The agent's picture: the uploaded `widget_bot` image, or the Ollie mark. */
export function BotAvatar({ src, size = 28, className }: { src?: string | null; size?: number; className?: string }) {
  if (src)
    return <img src={src} alt="" aria-hidden className={cn("shrink-0 rounded-full object-cover", className)} style={{ width: size, height: size }} />;
  return (
    <span aria-hidden className={cn("inline-flex shrink-0 items-center justify-center", className)} style={{ width: size, height: size }}>
      <OllieMark size={size} eyes={size >= 16} />
    </span>
  );
}

/** "1,234 / 4,000", red past the limit. */
export function Counter({ n, max, className }: { n: number; max: number; className?: string }) {
  return (
    <span className={cn("mono text-[11px] tabular-nums", n > max ? "text-danger-fg" : n > max * 0.9 ? "text-tone-amber" : "text-fg-4", className)} aria-live="polite">
      {number(n)} / {number(max)}
    </span>
  );
}

/** A violet strip that says the agent is off, with a way to the switch. */
export function AgentOffNotice({ className }: { className?: string }) {
  const { t } = useTranslation("agent");
  return (
    <div className={cn("flex animate-rise-in items-center gap-3 rounded-lg border px-4 py-3", tone[AI_TONE].soft, tone[AI_TONE].line, className)} role="status">
      <OllieMark size={22} tone="muted" />
      <div className="min-w-0 flex-1 text-[12.5px] leading-[18px]">
        <span className="font-medium text-fg">{t("off.title")}</span> <span className="text-fg-3">{t("off.body")}</span>
      </div>
      <Link to="/settings/ai/behaviour" className="inline-flex shrink-0 items-center gap-1 text-[12.5px] font-medium text-fg-2 hover:text-fg">
        {t("off.action")}
        <ArrowRight className="size-3.5" />
      </Link>
    </div>
  );
}

/**
 * An amount in US dollars as a friendly field: digits and a decimal point,
 * kept to cents, formatted while not focused, with quick picks underneath.
 * 0 reads as "No cap".
 */
export function UsdField({
  value,
  onChange,
  max,
  presets,
  zeroLabel,
  disabled,
  id,
  "aria-label": ariaLabel,
  "aria-describedby": describedBy,
}: {
  value: number;
  onChange: (n: number) => void;
  max: number;
  presets: number[];
  zeroLabel: string;
  disabled?: boolean;
  id?: string;
  "aria-label"?: string;
  "aria-describedby"?: string;
}) {
  const { t } = useTranslation("agent");
  const [text, setText] = useState<string | null>(null);
  const commit = (s: string) => {
    setText(null);
    const n = Number(s.replace(",", ".")) || 0;
    onChange(Math.min(max, Math.max(0, Math.round(n * 100) / 100)));
  };
  return (
    <div className="flex flex-col gap-2">
      <div
        className={cn(
          "flex h-8 w-[220px] items-center gap-2 rounded-md border border-line bg-surface pl-2.5 pr-2 shadow-xs transition-[border-color,box-shadow] duration-150 focus-within:border-focus-line focus-within:shadow-[0_0_0_3px_var(--focus)]",
          disabled && "opacity-60",
        )}
      >
        <span className="shrink-0 text-[12px] text-fg-3">$</span>
        <input
          id={id}
          type="text"
          inputMode="decimal"
          aria-label={ariaLabel}
          aria-describedby={describedBy}
          disabled={disabled}
          value={text ?? (value === 0 ? "" : number(value, { minimumFractionDigits: 2, maximumFractionDigits: 2 }))}
          placeholder={zeroLabel}
          onFocus={() => setText(value === 0 ? "" : String(value))}
          onChange={(e) => setText(e.target.value.replace(/[^\d.,]/g, "").replace(/([.,].*)[.,]/g, "$1").slice(0, 9))}
          onBlur={() => text !== null && commit(text)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && text !== null) {
              e.preventDefault();
              commit(text);
            }
          }}
          className="mono min-w-0 flex-1 bg-transparent text-[12.5px] tabular-nums text-fg outline-none placeholder:font-sans placeholder:text-fg-3"
        />
        <span className="shrink-0 text-[12px] text-fg-3">{t("usd.unit")}</span>
      </div>
      <div className="flex flex-wrap gap-1" role="group" aria-label={t("usd.presets")}>
        {[0, ...presets].map((p) => (
          <button
            key={p}
            type="button"
            disabled={disabled}
            aria-pressed={value === p}
            onClick={() => {
              setText(null);
              onChange(p);
            }}
            className={cn(
              "h-6 rounded-sm px-2 text-[11.5px] transition-colors disabled:opacity-50",
              value === p ? "bg-inverse text-inverse-fg" : "bg-hover text-fg-2 hover:bg-active hover:text-fg",
            )}
          >
            {p === 0 ? zeroLabel : formatUsd(p, { trim: true })}
          </button>
        ))}
      </div>
    </div>
  );
}

/** A label-caps eyebrow with an optional count, for small panels. */
export function Eyebrow({ children, count, className }: { children: ReactNode; count?: ReactNode; className?: string }) {
  return (
    <div className={cn("flex items-center gap-2", className)}>
      <span className="label-caps">{children}</span>
      {count !== undefined && <span className="mono text-[10.5px] tabular-nums text-fg-4">{count}</span>}
    </div>
  );
}
