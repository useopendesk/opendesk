// Picks the bot a widget or an address answers as (contracts/bots.md). The
// default bot stands for null: the widget follows whichever bot is the default.

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/select";
import type { Bot } from "@/lib/api/types";
import { q } from "@/lib/queries";
import { BotFace } from "./BotFace";

/** The bot a widget or an address answers as: its own, else the default. */
export function resolveBot(bots: Bot[] | undefined, id: string | null | undefined): Bot | undefined {
  return bots?.find((b) => b.id === id) ?? bots?.find((b) => b.is_default);
}

export function BotSelect({
  value,
  onChange,
  id,
  className,
  size,
  disabled,
  "aria-label": ariaLabel,
}: {
  value: string | null;
  onChange: (id: string | null) => void;
  id?: string;
  className?: string;
  size?: "sm" | "md";
  disabled?: boolean;
  "aria-label"?: string;
}) {
  const { t } = useTranslation("bots");
  const { data: bots } = useQuery(q.bots());
  const def = bots?.find((b) => b.is_default);
  const current = resolveBot(bots, value);
  return (
    <Select
      id={id}
      size={size}
      className={className}
      disabled={disabled || !bots}
      aria-label={ariaLabel ?? t("picker.label")}
      value={current?.id}
      placeholder={t("picker.loading")}
      onChange={(v) => onChange(v === def?.id ? null : v)}
      options={(bots ?? []).map((b) => ({
        value: b.id,
        // The face is part of the label so the closed picker shows it too (the select's svg size is 14px).
        label: (
          <span className="flex min-w-0 items-center gap-2">
            <BotFace look={b.look} image={b.image_url} size={14} />
            <span className="truncate">{b.name}</span>
          </span>
        ),
        hint: b.is_default ? t("list.default") : b.role || undefined,
      }))}
    />
  );
}
