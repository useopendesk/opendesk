import { cn } from "@/lib/cn";

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

export function modKey() {
  return isMac ? "⌘" : "Ctrl";
}

export function Kbd({ children, className }: { children: React.ReactNode; className?: string }) {
  const parts = typeof children === "string" ? children.replace("mod", modKey()).split(" ").map((k) => (k.length === 1 ? k.toUpperCase() : k)) : [children];
  return (
    <span className={cn("inline-flex items-center gap-0.5", className)}>
      {parts.map((p, i) => (
        <kbd
          key={i}
          className="inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-sm border border-line bg-surface px-1 font-mono text-[10.5px] leading-none text-fg-3 shadow-[0_1px_0_var(--line)]"
        >
          {p}
        </kbd>
      ))}
    </span>
  );
}
