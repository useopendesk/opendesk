// Settings > Plan and billing > Payment method and invoices (contracts/billing.md
// "Payment method and invoices"): the card top-ups and auto-recharge charge, and every
// recent invoice with its Stripe page and PDF. Owners and admins only.

import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, CreditCard, Download, Receipt } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Section } from "@/components/layout";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Billing, BillingInvoice } from "@/lib/api/types";
import { cn } from "@/lib/cn";
import { q } from "@/lib/queries";
import { tone } from "@/lib/tones";
import { useAddPaymentMethod, useCanManageBilling, useFmt } from "./hooks";
import { expiresSoon, formatMoney, paymentMethodExpiry, paymentMethodLabel } from "./model";

const SHOWN = 6;

export function PaymentAndInvoices({ b }: { b: Billing }) {
  const { t } = useTranslation("billing");
  const canManage = useCanManageBilling();
  const addCard = useAddPaymentMethod();
  if (!canManage || !b.billing_enabled) return null;
  const pm = b.payment_method ?? null;
  const expiry = pm ? paymentMethodExpiry(pm) : null;
  const soon = !!pm && expiresSoon(pm);

  return (
    <div id="invoices" className="scroll-mt-6">
      <Section title={t("invoices.title")} description={t("invoices.description")} flush>
        <div className="flex flex-wrap items-center gap-3 px-5 py-3.5">
          <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-md border border-line bg-sunken text-fg-3")} aria-hidden>
            <CreditCard className="size-4" strokeWidth={1.75} />
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[13px] font-medium">{pm ? paymentMethodLabel(pm) : t("invoices.noCard")}</div>
            <div className={cn("mt-0.5 text-[11.5px]", soon ? tone.amber.text : "text-fg-3")}>
              {pm ? (expiry ? (soon ? t("invoices.expiresSoon", { date: expiry }) : t("invoices.expires", { date: expiry })) : t("invoices.used")) : t("invoices.noCardHint")}
            </div>
          </div>
          <Button variant={pm ? "secondary" : "primary"} loading={addCard.isPending} onClick={() => addCard.mutate()}>
            {pm ? t("invoices.change") : t("invoices.addCard")}
          </Button>
        </div>
        <InvoiceList />
      </Section>
    </div>
  );
}

function InvoiceList() {
  const { t } = useTranslation("billing");
  const [all, setAll] = useState(false);
  const { data, isPending, isError, refetch } = useQuery(q.billingInvoices());

  if (isPending)
    return (
      <div className="flex flex-col gap-2 border-t border-line-soft px-5 py-3.5" aria-busy>
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-5 w-full" />
        ))}
      </div>
    );
  if (isError)
    return (
      <div className="flex items-center justify-between gap-3 border-t border-line-soft px-5 py-3 text-[12px] text-fg-3">
        {t("invoices.error")}
        <Button size="xs" onClick={() => void refetch()}>
          {t("retry")}
        </Button>
      </div>
    );
  if (data.length === 0)
    return (
      <div className="border-t border-line-soft">
        <EmptyState compact icon={<Receipt className="size-5 text-fg-4" strokeWidth={1.5} />} title={t("invoices.empty")} description={t("invoices.emptyBody")} />
      </div>
    );
  const shown = all ? data : data.slice(0, SHOWN);
  return (
    <div className="border-t border-line-soft">
      <ul>
        {shown.map((inv) => (
          <InvoiceRow key={inv.id} inv={inv} />
        ))}
      </ul>
      {data.length > SHOWN && (
        <div className="border-t border-line-soft px-5 py-2">
          <Button size="xs" variant="ghost" onClick={() => setAll(!all)}>
            {all ? t("invoices.fewer") : t("invoices.more", { count: data.length - SHOWN })}
          </Button>
        </div>
      )}
    </div>
  );
}

function InvoiceRow({ inv }: { inv: BillingInvoice }) {
  const { t } = useTranslation("billing");
  const fmt = useFmt();
  const open = inv.status === "open";
  return (
    <li className="flex items-center gap-3 border-b border-line-soft px-5 py-2.5 last:border-0">
      <span className="mono w-[64px] shrink-0 text-[11.5px] text-fg-3 tabular-nums">{fmt.date(inv.created)}</span>
      <div className="min-w-0 flex-1">
        <div className="truncate text-[12.5px] text-fg">{t(`invoices.kind.${inv.kind}`)}</div>
        {inv.number && <div className="mono truncate text-[11px] text-fg-4">{inv.number}</div>}
      </div>
      <Badge tone={inv.status === "paid" ? "ok" : open ? "warn" : "danger"}>{t(`invoices.status.${inv.status}`)}</Badge>
      <span className="mono w-[76px] shrink-0 text-right text-[12.5px] tabular-nums">{formatMoney(inv.total, inv.currency, fmt.locale)}</span>
      <div className="flex w-[64px] shrink-0 justify-end gap-0.5">
        {inv.pdf_url && (
          <Button size="xs" variant="ghost" square asChild aria-label={t("invoices.download", { number: inv.number })}>
            <a href={inv.pdf_url} target="_blank" rel="noreferrer">
              <Download />
            </a>
          </Button>
        )}
        {inv.url && (
          <Button size="xs" variant="ghost" square asChild aria-label={open ? t("invoices.pay", { number: inv.number }) : t("invoices.view", { number: inv.number })}>
            <a href={inv.url} target="_blank" rel="noreferrer">
              <ArrowUpRight />
            </a>
          </Button>
        )}
      </div>
    </li>
  );
}
