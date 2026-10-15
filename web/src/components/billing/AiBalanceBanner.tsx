// A calm strip at the top of the work panel when the AI balance is out or running
// low and auto-recharge won't refill them (contracts/billing.md "Auto-recharge").
// Owners and admins only; dismissed per state and period; hidden on the billing
// page, which says the same in place.

import { useQuery } from "@tanstack/react-query";
import { useNavigate, useRouterState } from "@tanstack/react-router";
import { Sparkles, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { q } from "@/lib/queries";
import { useSession } from "@/lib/session";
import { AI_TONE, tone } from "@/lib/tones";
import { useCanManageBilling, useFmt } from "./hooks";
import { balanceBanner } from "./model";

function wasDismissed(key: string) {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

export function AiBalanceBanner() {
  const { t } = useTranslation("billing");
  const fmt = useFmt();
  const navigate = useNavigate();
  const canManage = useCanManageBilling();
  const wid = useSession((s) => s.workspaceId);
  const onBilling = useRouterState({ select: (s) => s.location.pathname.startsWith("/settings/billing") });
  const { data: b } = useQuery({ ...q.billing(), enabled: canManage });
  const [dismissed, setDismissed] = useState<string | null>(null);

  const kind = canManage ? balanceBanner(b) : null;
  if (!kind || !b || onBilling) return null;
  const key = `od-ai-balance-banner:${wid}:${kind}:${b.ai.period_end}`;
  if (dismissed === key || wasDismissed(key)) return null;
  const ar = b.auto_recharge;
  const dismiss = () => {
    setDismissed(key);
    try {
      localStorage.setItem(key, "1");
    } catch {
      /* private mode: hidden until reload */
    }
  };
  const hue = kind === "out" ? tone.amber : tone[AI_TONE];

  return (
    <div role="status" className={cn("flex shrink-0 animate-rise-in flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-4 py-2 motion-reduce:animate-none", hue.soft, hue.line)}>
      <Sparkles className={cn("size-3.5 shrink-0", hue.text)} aria-hidden />
      <p className="min-w-0 flex-1 text-[12.5px] leading-[18px]">
        <span className="font-medium">{kind === "out" ? t("banner.outTitle") : t("banner.lowTitle")}</span>{" "}
        <span className="text-fg-3">
          {kind === "out" ? t("banner.outBody") : t("banner.lowBody", { remaining: fmt.usd(b.ai.remaining_usd, { tiny: true }) })}
          {ar?.failure && !ar.enabled ? ` ${t("banner.failed")}` : ""}
        </span>
      </p>
      <div className="flex shrink-0 items-center gap-1.5">
        <Button size="xs" variant="primary" onClick={() => void navigate({ to: "/settings/billing", hash: "ai-balance" })}>
          {t("banner.buy")}
        </Button>
        {b.prices.ai_top_up && ar && (
          <Button size="xs" onClick={() => void navigate({ to: "/settings/billing", hash: "auto-recharge" })}>
            {ar.enabled ? t("banner.settings") : t("banner.turnOn")}
          </Button>
        )}
        <Button size="xs" variant="ghost" square icon={<X />} aria-label={t("banner.dismiss")} onClick={dismiss} />
      </div>
    </div>
  );
}
