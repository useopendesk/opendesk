// Small shared pieces: an id with a copy button, compact key/values for audit
// data, and a external link.

import { ExternalLink } from "lucide-react";
import type { ReactNode } from "react";
import { CopyButton } from "@/components/ui/copy-button";
import { cn } from "@/lib/cn";

export function IdChip({ value, label, className }: { value: string; label?: string; className?: string }) {
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-0.5 rounded-sm bg-sunken py-px pl-1.5 text-[11.5px] text-fg-2 shadow-[inset_0_0_0_1px_var(--line-soft)]", className)}>
      {label && <span className="mr-1 text-fg-4">{label}</span>}
      <span className="mono truncate">{value}</span>
      <CopyButton value={value} label={`Copy ${label ?? "id"}`} className="size-5" />
    </span>
  );
}

function show(v: unknown): string {
  if (v === null || v === undefined) return "none";
  if (typeof v === "string") return v;
  if (typeof v === "number" || typeof v === "boolean") return String(v);
  try {
    return JSON.stringify(v);
  } catch {
    return String(v);
  }
}

/** An object's members as compact "key value" chips, for audit data. */
export function DataPairs({ data, max = 6 }: { data: Record<string, unknown> | null | undefined; max?: number }) {
  const entries = Object.entries(data ?? {});
  if (!entries.length) return <span className="text-fg-4">–</span>;
  const shown = entries.slice(0, max);
  return (
    <span className="flex flex-wrap gap-1">
      {shown.map(([k, v]) => {
        const text = show(v);
        return (
          <span key={k} title={`${k}: ${text}`} className="inline-flex max-w-[260px] items-center gap-1 rounded-sm bg-hover px-1.5 py-px text-[11px]">
            <span className="text-fg-3">{k}</span>
            <span className="mono truncate text-fg">{text}</span>
          </span>
        );
      })}
      {entries.length > max && <span className="text-[11px] text-fg-4">+{entries.length - max} more</span>}
    </span>
  );
}

export function ExtLink({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" className={cn("inline-flex items-center gap-1 text-fg underline decoration-line-strong underline-offset-2 hover:decoration-fg", className)}>
      {children}
      <ExternalLink className="size-3 shrink-0 text-fg-3" aria-hidden />
    </a>
  );
}

/** "Not set" in quiet grey, for empty values in key/value lists. */
export function None({ children = "None" }: { children?: ReactNode }) {
  return <span className="text-fg-4">{children}</span>;
}
