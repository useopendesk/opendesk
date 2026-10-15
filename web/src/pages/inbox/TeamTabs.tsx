// The tabs of a team inbox: Unassigned, Open, Pending, All. Arrow keys move
// between them; the underline glides (or jumps, with reduced motion).

import { LayoutGroup, motion, useReducedMotion } from "motion/react";
import { useId, useRef } from "react";
import { useTranslation } from "react-i18next";
import { teamTabs, type TeamTab } from "@/lib/folders";
import { springs } from "@/lib/motion";
import { cn } from "@/lib/cn";

export function TeamTabs({
  tab,
  counts,
  onTab,
}: {
  tab: TeamTab;
  counts?: { open: number; unassigned: number };
  onTab: (t: TeamTab) => void;
}) {
  const { t } = useTranslation("inbox");
  const reduce = useReducedMotion();
  const id = useId();
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const count = (x: TeamTab) => (x === "unassigned" ? counts?.unassigned : x === "open" ? counts?.open : undefined);
  return (
    <LayoutGroup id={id}>
      <div role="tablist" aria-label={t("teams.tabs")} className="flex h-9 items-end gap-0.5 px-2.5">
        {teamTabs.map((x, i) => {
          const active = x === tab;
          const n = count(x);
          return (
            <button
              key={x}
              ref={(el) => {
                refs.current[i] = el;
              }}
              type="button"
              role="tab"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              onClick={() => onTab(x)}
              onKeyDown={(e) => {
                const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
                if (!d) return;
                e.preventDefault();
                const next = (i + d + teamTabs.length) % teamTabs.length;
                refs.current[next]?.focus();
                onTab(teamTabs[next]);
              }}
              className={cn(
                "relative flex h-8 items-center gap-1.5 rounded-t-sm px-2 text-[12.5px] outline-none transition-colors focus-visible:shadow-[0_0_0_2px_var(--focus)]",
                active ? "font-medium text-fg" : "text-fg-3 hover:text-fg",
              )}
            >
              {t(`teams.tab.${x}`)}
              {n !== undefined && n > 0 && <span className="mono text-[10.5px] tabular-nums text-fg-4">{n}</span>}
              {active && (
                <motion.span
                  layoutId={reduce ? undefined : "team-tab"}
                  transition={springs.glide}
                  className="absolute inset-x-1.5 -bottom-px h-[1.5px] rounded-full bg-fg"
                />
              )}
            </button>
          );
        })}
      </div>
    </LayoutGroup>
  );
}
