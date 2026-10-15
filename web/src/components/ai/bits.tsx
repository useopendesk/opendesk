// Shared AI affordances. AI is marked by a small violet sparkle (AI_TONE, so
// every AI surface reads as one family) and a quiet cost in US dollars in the tooltip.

import { Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { AIFeature } from "@/lib/api/types";
import { useAI } from "@/lib/ai";
import { cn } from "@/lib/cn";
import { formatUsd } from "@/lib/format";
import { AI_TONE, tone } from "@/lib/tones";
import { Tooltip } from "@/components/ui/tooltip";

export function AiIcon({ className }: { className?: string }) {
  return <Sparkles aria-hidden className={cn("size-3.5 shrink-0", tone[AI_TONE].text, className)} strokeWidth={1.75} />;
}

/** "$0.0135", or "about $0.0135" when each call is charged on its real cost (the figure is then typical). */
export function useCostText() {
  const { t } = useTranslation();
  const { config } = useAI();
  return (usd: number) => {
    const cost = formatUsd(usd, { precise: true });
    return config?.metered ? t("aboutCost", { cost }) : cost;
  };
}

/** "about $0.0135", for tooltips and menus. Empty when the cost is unknown or AI is unlimited. */
export function useCostLabel() {
  const { cost } = useAI();
  const text = useCostText();
  return (feature: AIFeature) => {
    const n = cost(feature);
    return n === null ? "" : text(n);
  };
}

/** Wraps an AI control with a tooltip naming the action and what it typically costs. */
export function AiTooltip({ feature, label, children }: { feature: AIFeature; label: ReactNode; children: ReactNode }) {
  const cost = useCostLabel()(feature);
  return (
    <Tooltip
      content={
        <span className="flex items-center gap-2">
          <span>{label}</span>
          {cost && <span className="mono text-[10.5px] opacity-60">{cost}</span>}
        </span>
      }
    >
      {children}
    </Tooltip>
  );
}
