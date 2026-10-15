// The one upgrade prompt for 402 plan_limit and ai_balance_empty, wherever they
// happen. Calm: it explains the limit, then offers the next step.

import { Link } from "@tanstack/react-router";
import { Check, Sparkles } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useUpgrade, upgrade } from "@/lib/upgrade";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { useBilling, useBuyTopUps, useCanManageBilling, useCheckout, useFmt, useTopUp } from "./hooks";
import { DEFAULT_TOP_UP_USD, formatPrice, paymentMethodLabel } from "./model";

export default function UpgradeDialog() {
  const { t } = useTranslation("billing");
  const reason = useUpgrade((s) => s.reason);
  const { data: b } = useBilling();
  const canManage = useCanManageBilling();
  const checkout = useCheckout();
  const buy = useBuyTopUps();
  const pay = useTopUp();
  const fmt = useFmt();
  if (!reason) return null;

  const balance = reason.code === "ai_balance_empty";
  const month = b?.prices.pro_month ?? null;
  const topUp = b?.prices.ai_top_up ?? null;
  const onFree = !b || b.plan === "free";
  const price = month ? formatPrice(month, fmt.locale) : null;

  const body = balance
    ? b?.ai.period_end
      ? t("dialog.balanceBody", { date: fmt.date(b.ai.period_end), remaining: fmt.usd(b.ai.remaining_usd, { tiny: true }) })
      : t("dialog.balanceBodyNoDate")
    : price
      ? t("dialog.limitBody", { price })
      : t("dialog.limitBodyNoPrice");

  const close = () => upgrade.close();
  const busy = checkout.isPending || buy.isPending || pay.isPending;
  const pm = b?.payment_method ?? null;
  // With a saved card the AI usage is added here and now; otherwise in Stripe Checkout.
  const addUsage = () => (pm ? pay.mutate(DEFAULT_TOP_UP_USD, { onSuccess: close }) : buy.mutate(DEFAULT_TOP_UP_USD));
  const usd = fmt.usd(DEFAULT_TOP_UP_USD, { trim: true });

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && close()}
      size="sm"
      title={balance ? t("dialog.balanceTitle") : t("dialog.limitTitle")}
      footer={
        canManage && b?.billing_enabled ? (
          <>
            <Button variant="ghost" asChild onClick={close}>
              <Link to="/settings/billing">{t("dialog.seePlans")}</Link>
            </Button>
            {balance && topUp && (
              <Button variant={onFree ? "secondary" : "primary"} loading={buy.isPending || pay.isPending} disabled={busy} onClick={addUsage}>
                {pm ? t("dialog.buyTopUpCard", { usd, card: paymentMethodLabel(pm) }) : t("dialog.buyTopUp", { usd })}
              </Button>
            )}
            {onFree && (
              <Button variant="primary" loading={checkout.isPending} disabled={busy} onClick={() => checkout.mutate("month")}>
                {t("upgrade.cta")}
              </Button>
            )}
          </>
        ) : (
          <Button variant="primary" onClick={close}>
            {t("dialog.notNow")}
          </Button>
        )
      }
    >
      <div className="flex flex-col gap-3.5 pb-1">
        {!balance && reason.detail && <p className="text-[13px] leading-5 text-fg">{reason.detail}</p>}
        <p className="text-[12.5px] leading-[19px] text-fg-3">{body}</p>
        {balance && !onFree && <p className="text-[12.5px] text-fg-3">{t("dialog.balancePro")}</p>}
        {onFree && (
          <ul className="flex flex-col gap-1.5 rounded-md border border-line bg-sunken/50 px-3 py-2.5">
            {(["mailboxes", "members", "ai"] as const).map((k) => (
              <li key={k} className="flex items-center gap-2 text-[12.5px] text-fg-2">
                {k === "ai" ? <Sparkles className="size-3.5 text-fg-3" strokeWidth={1.75} /> : <Check className="size-3.5 text-fg-3" strokeWidth={2} />}
                {t(`upgrade.benefit.${k}`)}
              </li>
            ))}
          </ul>
        )}
        {(!canManage || !b?.billing_enabled) && <p className="text-[12.5px] text-fg-3">{balance ? t("dialog.askAdminBalance") : t("dialog.askAdmin")}</p>}
      </div>
    </Dialog>
  );
}
