// Settings > Plan and billing > AI balance > Add AI usage (contracts/billing.md "AI top-ups").
// Any amount from $5 to $5,000: a few presets or a typed one. With a saved card it's paid
// in place after one confirm; without one, or when the bank wants the buyer to confirm,
// it carries on in Stripe Checkout.

import { CreditCard } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { TextInput } from "@/components/ui/input";
import { Segmented } from "@/components/ui/tabs";
import type { Billing } from "@/lib/api/types";
import { useBuyTopUps, useFmt, useTopUp } from "./hooks";
import { DEFAULT_TOP_UP_USD, dollarForDollar, MAX_TOP_UP_USD, MIN_TOP_UP_USD, parseUsd, paymentMethodLabel, TOP_UP_PRESETS, topUpQuote, validTopUp } from "./model";

type Choice = `${(typeof TOP_UP_PRESETS)[number]}` | "custom";

export function TopUpPanel({ b, initial = DEFAULT_TOP_UP_USD }: { b: Billing; initial?: number }) {
  const { t } = useTranslation("billing");
  const fmt = useFmt();
  const confirm = useConfirm();
  const checkout = useBuyTopUps();
  const pay = useTopUp();
  const preset = TOP_UP_PRESETS.find((v) => v === initial);
  const [choice, setChoice] = useState<Choice>(preset ? (`${preset}` as Choice) : "custom");
  const [custom, setCustom] = useState(preset ? "" : String(initial));
  const topUp = b.prices.ai_top_up;
  if (!topUp) return null;

  const pm = b.payment_method ?? null;
  const usd = choice === "custom" ? parseUsd(custom) : Number(choice);
  const valid = validTopUp(usd);
  const quote = valid ? topUpQuote(topUp, usd, fmt.locale) : null;
  const same = dollarForDollar(topUp);
  const busy = pay.isPending || checkout.isPending;
  const amount = (n: number) => fmt.usd(n, { trim: true });
  const typedWrong = choice === "custom" && custom.trim() !== "" && !valid;

  const buy = () => {
    if (!valid || !quote) return;
    if (!pm) {
      checkout.mutate(usd);
      return;
    }
    confirm.show(
      t("topUp.confirmBody", { usd: amount(usd), price: quote.price ?? amount(usd), card: paymentMethodLabel(pm) }),
      // The hook reports a decline itself; the dialog just closes.
      () => pay.mutateAsync(usd).catch(() => undefined),
      { title: t("topUp.confirmTitle", { usd: amount(usd) }), confirmLabel: t("topUp.confirmCta", { price: quote.price ?? amount(usd) }) },
    );
  };

  const summary = typedWrong
    ? t("topUp.range", { min: amount(MIN_TOP_UP_USD), max: amount(MAX_TOP_UP_USD) })
    : !quote
      ? t("topUp.enterAmount")
      : same
        ? t("topUp.sameAmount", { usd: amount(usd!) })
        : t("topUp.total", { usd: amount(usd!), price: quote.price ?? "" });

  return (
    <div className="border-t border-line-soft bg-sunken/50 px-5 py-3.5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
        <div className="text-[13px] font-medium">{t("topUp.title")}</div>
        <div className="text-[11.5px] text-fg-3">{t("topUp.hint")}</div>
      </div>
      <div className="mt-2.5 flex flex-wrap items-center gap-2">
        <Segmented<Choice>
          value={choice}
          onChange={setChoice}
          options={[...TOP_UP_PRESETS.map((v) => ({ value: `${v}` as Choice, label: <span className="tabular-nums">{amount(v)}</span> })), { value: "custom", label: t("topUp.other") }]}
        />
        {choice === "custom" && (
          <TextInput
            autoFocus
            inputSize="sm"
            inputMode="decimal"
            value={custom}
            onChange={(e) => setCustom(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && buy()}
            leading={<span className="text-fg-4">$</span>}
            placeholder={String(DEFAULT_TOP_UP_USD)}
            invalid={typedWrong}
            aria-label={t("topUp.amount")}
            className="w-[112px] tabular-nums"
          />
        )}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2">
        <p className={typedWrong ? "min-w-0 flex-1 text-[12px] text-danger-fg" : "min-w-0 flex-1 text-[12px] text-fg-3"} aria-live="polite">
          {summary}
        </p>
        {pm && (
          <Button variant="ghost" disabled={!valid || busy} loading={checkout.isPending} onClick={() => valid && checkout.mutate(usd)}>
            {t("topUp.otherWay")}
          </Button>
        )}
        <Button variant="primary" icon={pm ? <CreditCard /> : undefined} disabled={!valid || busy} loading={pay.isPending || (!pm && checkout.isPending)} onClick={buy}>
          {quote ? (pm ? t("topUp.pay", { price: quote.price }) : t("topUp.continue", { price: quote.price })) : t("topUp.payEmpty")}
        </Button>
      </div>
      {pm && <p className="mono mt-1.5 text-right text-[11px] text-fg-4">{t("topUp.charged", { card: paymentMethodLabel(pm) })}</p>}
    </div>
  );
}
