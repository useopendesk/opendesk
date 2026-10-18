import { cn } from "@/lib/cn";

/** A row of filter chips: one is on. Counts are optional. Arrow keys move between them. */
export function FilterChips<V extends string>({
  value,
  onChange,
  options,
  label,
  className,
}: {
  value: V;
  onChange: (v: V) => void;
  options: { value: V; label: string; count?: number; tone?: string }[];
  label: string;
  className?: string;
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className={cn("flex flex-wrap items-center gap-1", className)}
      onKeyDown={(e) => {
        if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
        const i = options.findIndex((o) => o.value === value);
        const next = options[(i + (e.key === "ArrowRight" ? 1 : options.length - 1)) % options.length];
        onChange(next.value);
        (e.currentTarget.querySelector(`[data-value="${next.value}"]`) as HTMLElement | null)?.focus();
        e.preventDefault();
      }}
    >
      {options.map((o) => {
        const on = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={on}
            data-value={o.value}
            tabIndex={on ? 0 : -1}
            onClick={() => onChange(o.value)}
            className={cn(
              "inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium transition-colors duration-100",
              on ? "bg-inverse text-inverse-fg" : "text-fg-2 shadow-[inset_0_0_0_1px_var(--line)] hover:bg-hover hover:text-fg",
            )}
          >
            {o.label}
            {o.count !== undefined && <span className={cn("mono text-[10.5px] tabular-nums", on ? "opacity-70" : "text-fg-4")}>{o.count}</span>}
          </button>
        );
      })}
    </div>
  );
}
