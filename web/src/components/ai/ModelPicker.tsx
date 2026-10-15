// The model choice for AI settings and the widget agent: a popover listbox of
// the models the instance offers (GET /v1/ai/models), each with its vendor,
// context length, price per million tokens in US dollars and what a typical
// draft reply costs on it, plus "Default", which follows
// the instance (or another setting) and names the model that means today.
// Keyboard: arrows move, Enter picks, Escape and click-away close.

import { Command } from "cmdk";
import { Popover as P } from "radix-ui";
import { Check, ChevronDown } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { AIModel } from "@/lib/api/types";
import { compact, formatUsd } from "@/lib/format";
import { cn } from "@/lib/cn";
import { tone, type Tone } from "@/lib/tones";
import { floatingPanel } from "@/components/ui/popover";
import { pickerSearch } from "@/components/ui/picker";

const VENDOR_TONE: Record<string, Tone> = { anthropic: "orange", openai: "green", google: "blue", deepseek: "indigo", meta: "blue", mistral: "amber" };

function vendorOf(m: Pick<AIModel, "id" | "vendor">): string {
  if (m.vendor) return m.vendor;
  const slash = m.id.indexOf("/");
  return slash > 0 ? m.id.slice(0, slash) : "";
}

/** A small tile with the vendor's initial in a stable hue. */
export function VendorMark({ vendor, className }: { vendor: string; className?: string }) {
  const hue = VENDOR_TONE[vendor.toLowerCase()] ?? "gray";
  return (
    <span aria-hidden className={cn("flex size-5 shrink-0 items-center justify-center rounded-[5px] text-[10px] font-semibold uppercase", tone[hue].chip, className)}>
      {vendor ? vendor.charAt(0) : "?"}
    </span>
  );
}

/** "Claude Sonnet 5.5", or the id when the model isn't in the list. */
export function modelName(id: string, models: AIModel[] | undefined): string {
  return models?.find((m) => m.id === id)?.name || id;
}

/** "$3 / $15 per 1M" (input / output tokens), or null when the price is unknown (or on the workspace's own key). */
export function useModelPrice() {
  const { t } = useTranslation("agent");
  return (m: AIModel | undefined) =>
    m?.usd_per_mtok ? t("model.price", { input: formatUsd(m.usd_per_mtok.input, { trim: true }), output: formatUsd(m.usd_per_mtok.output, { trim: true }) }) : null;
}

/** "Draft reply about $0.0195", or null when the price is unknown. */
export function useTypicalReply() {
  const { t } = useTranslation("agent");
  return (m: AIModel | undefined) => (m?.typical_reply_usd != null ? t("model.typicalReply", { cost: formatUsd(m.typical_reply_usd, { precise: true }) }) : null);
}

export function ModelPicker({
  value,
  onChange,
  models,
  defaultId,
  defaultLabel,
  disabled,
  id,
  "aria-label": ariaLabel,
  className,
}: {
  /** "" means the default. */
  value: string;
  onChange: (id: string) => void;
  models: AIModel[] | undefined;
  /** The model "Default" resolves to right now. */
  defaultId: string;
  /** The default row's label, e.g. "Default" or "Same as the main model". */
  defaultLabel?: string;
  disabled?: boolean;
  id?: string;
  "aria-label"?: string;
  className?: string;
}) {
  const { t } = useTranslation("agent");
  const [open, setOpen] = useState(false);
  const price = useModelPrice();
  const typical = useTypicalReply();
  const list = models ?? [];
  const current = value ? list.find((m) => m.id === value) : undefined;
  const fallback = list.find((m) => m.id === defaultId);
  const shown = current ?? (value ? undefined : fallback);
  const label = defaultLabel ?? t("model.default");
  const pick = (v: string) => {
    onChange(v);
    setOpen(false);
  };

  return (
    <P.Root open={open} onOpenChange={setOpen}>
      <P.Trigger asChild disabled={disabled}>
        <button
          id={id}
          type="button"
          aria-label={ariaLabel}
          aria-haspopup="listbox"
          className={cn(
            "@container group inline-flex h-9 w-full min-w-0 items-center gap-2.5 rounded-md border border-line bg-surface pl-2 pr-2.5 text-left shadow-xs outline-none transition-[border-color,box-shadow] duration-150 hover:border-line-strong focus-visible:border-focus-line focus-visible:shadow-[0_0_0_3px_var(--focus)] disabled:pointer-events-none disabled:opacity-60",
            className,
          )}
        >
          <VendorMark vendor={shown ? vendorOf(shown) : ""} />
          <span className="min-w-0 flex-1 truncate text-[12.5px]">
            {!value ? (
              <>
                <span className="font-medium text-fg">{label}</span>
                <span className="text-fg-3"> · {modelName(defaultId, list)}</span>
              </>
            ) : (
              <span className="font-medium text-fg">{current?.name || value}</span>
            )}
          </span>
          {shown && price(shown) && (
            <span className="mono hidden shrink-0 text-[10.5px] text-fg-4 @sm:inline" title={t("model.priceHint")}>
              {price(shown)}
            </span>
          )}
          <ChevronDown className="size-3.5 shrink-0 text-fg-3 transition-transform duration-150 group-data-[state=open]:rotate-180 motion-reduce:transition-none" />
        </button>
      </P.Trigger>
      <P.Portal>
        <P.Content
          align="start"
          sideOffset={6}
          collisionPadding={8}
          className={cn(floatingPanel, "w-[min(420px,calc(100vw-24px))] overflow-hidden p-0")}
          onClick={(e) => e.stopPropagation()}
        >
          <Command loop value={value || "__default"} label={ariaLabel ?? t("model.choose")} className="flex flex-col">
            {list.length > 12 && (
              // A provider key can list hundreds of models: search by id, name or vendor.
              <div className={pickerSearch.row}>
                <Command.Input autoFocus placeholder={t("model.search")} className={pickerSearch.input} />
              </div>
            )}
            <Command.List className="max-h-[340px] overflow-y-auto p-1">
              <Command.Empty className="px-2 py-5 text-center text-xs text-fg-3">{t("model.none")}</Command.Empty>
              <Row
                value="__default"
                selected={!value}
                onSelect={() => pick("")}
                icon={<VendorMark vendor={fallback ? vendorOf(fallback) : ""} />}
                title={label}
                meta={<span className="truncate">{modelName(defaultId, list)}</span>}
                price={price(fallback)}
                typical={typical(fallback)}
              />
              {list.length > 0 && <div className="mx-1 my-1 h-px bg-line-soft" />}
              {list.map((m) => (
                <Row
                  key={m.id}
                  value={m.id}
                  keywords={[m.name, vendorOf(m)]}
                  selected={value === m.id}
                  onSelect={() => pick(m.id)}
                  icon={<VendorMark vendor={vendorOf(m)} />}
                  title={m.name || m.id}
                  meta={
                    <>
                      {vendorOf(m) && <span>{vendorOf(m)}</span>}
                      {m.context_length ? <span>{t("model.context", { n: compact(m.context_length) })}</span> : null}
                    </>
                  }
                  price={price(m)}
                  typical={typical(m)}
                  badge={m.id === defaultId ? t("model.defaultBadge") : undefined}
                />
              ))}
              {value && !current && (
                <Row value={value} selected onSelect={() => pick(value)} icon={<VendorMark vendor="" />} title={value} meta={<span>{t("model.unavailable")}</span>} price={null} />
              )}
            </Command.List>
          </Command>
        </P.Content>
      </P.Portal>
    </P.Root>
  );
}

function Row({
  value,
  selected,
  onSelect,
  icon,
  title,
  meta,
  price,
  typical,
  badge,
  keywords,
}: {
  value: string;
  keywords?: string[];
  selected: boolean;
  onSelect: () => void;
  icon: ReactNode;
  title: ReactNode;
  meta: ReactNode;
  price: string | null;
  typical?: string | null;
  badge?: string;
}) {
  const { t } = useTranslation("agent");
  return (
    <Command.Item
      value={value}
      keywords={keywords}
      onSelect={onSelect}
      className="flex cursor-default items-center gap-2.5 rounded-md px-2 py-1.5 text-fg outline-none data-[selected=true]:bg-hover"
    >
      {icon}
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className="truncate text-[12.5px] font-medium">{title}</span>
          {badge && <span className="shrink-0 rounded-xs bg-hover px-1 text-[10px] text-fg-3">{badge}</span>}
        </span>
        <span className="mt-px flex min-w-0 items-center gap-1.5 text-[11px] text-fg-3 [&>span+span]:before:mr-1.5 [&>span+span]:before:content-['·']">{meta}</span>
      </span>
      {(price || typical) && (
        <span className="flex shrink-0 flex-col items-end text-right text-[10.5px] leading-4" title={price ? t("model.priceHint") : undefined}>
          {price && <span className="mono text-fg-3">{price}</span>}
          {typical && <span className="text-fg-4">{typical}</span>}
        </span>
      )}
      <Check className={cn("size-3.5 shrink-0", selected ? "text-fg" : "invisible")} />
    </Command.Item>
  );
}
