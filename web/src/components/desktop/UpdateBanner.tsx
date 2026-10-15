// The desktop app's own update, in a calm strip at the top of the work panel: a
// quiet progress line while it downloads, then "opendesk 1.3.0 is ready. Restart
// to update." with What's new (contracts/native.md, "Desktop shell"). The shell
// downloads in the background and installs on quit anyway, so the strip can be
// put away. Nothing in a browser, or in a desktop build without updates.

import { ArrowUpRight, Download, RefreshCw, X } from "lucide-react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { desktop, type DesktopUpdateState } from "@/lib/desktop";
import { dur, ease } from "@/lib/motion";
import { tone } from "@/lib/tones";

// The bridge hands out a fresh copy on every read, so the last state is kept
// here and React reads that one.
const idle: DesktopUpdateState = { status: "idle" };
let current: DesktopUpdateState | null = null;
const listeners = new Set<() => void>();
let unsubscribe: (() => void) | null = null;

function subscribe(cb: () => void) {
  const u = desktop?.update;
  if (!u) return () => {};
  listeners.add(cb);
  unsubscribe ??= u.onChange((s) => {
    current = s;
    listeners.forEach((l) => l());
  });
  return () => {
    listeners.delete(cb);
    if (!listeners.size) {
      unsubscribe?.();
      unsubscribe = null;
      current = null;
    }
  };
}

function snapshot(): DesktopUpdateState {
  current ??= desktop?.update?.state() ?? idle;
  return current;
}

export function useDesktopUpdate(): DesktopUpdateState {
  return useSyncExternalStore(subscribe, snapshot, () => idle);
}

export function DesktopUpdateBanner() {
  const { t } = useTranslation("shell");
  const s = useDesktopUpdate();
  const reduce = useReducedMotion();
  // Put away per version and step: a download put away still says when it's ready.
  const [dismissed, setDismissed] = useState<string | null>(null);
  const [restarting, setRestarting] = useState(false);

  const shown = s.status === "downloading" || s.status === "ready" ? s : null;
  const key = shown ? `${shown.status}:${shown.version}` : null;
  const open = !!shown && dismissed !== key;

  return (
    <AnimatePresence initial={false}>
      {open && shown && (
        <motion.div
          key="desktop-update"
          initial={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
          animate={reduce ? { opacity: 1 } : { opacity: 1, height: "auto" }}
          exit={reduce ? { opacity: 0 } : { opacity: 0, height: 0 }}
          transition={{ duration: dur.slow, ease: ease.out }}
          className="shrink-0 overflow-hidden"
        >
          <div
            role="status"
            className={cn(
              "relative flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-4 py-2 transition-colors duration-200 motion-reduce:transition-none",
              shown.status === "ready" ? cn(tone.blue.soft, tone.blue.line) : "border-line bg-sunken",
            )}
          >
            {shown.status === "ready" ? (
              <RefreshCw className={cn("size-3.5 shrink-0", tone.blue.text)} aria-hidden />
            ) : (
              <Download className="size-3.5 shrink-0 text-fg-3" aria-hidden />
            )}
            <p className="min-w-0 flex-1 text-[12.5px] leading-[18px]">
              {shown.status === "ready" ? (
                <>
                  <span className="font-medium">{t("desktop.updateReady", { version: shown.version })}</span>{" "}
                  <span className="text-fg-3">{t("desktop.updateReadyHint")}</span>
                </>
              ) : (
                <span className="text-fg-3">
                  {shown.version ? t("desktop.updateDownloading", { version: shown.version }) : t("desktop.updateDownloadingPlain")}
                  <span className="ml-2 font-mono text-[11.5px] tabular-nums">{shown.percent}%</span>
                </span>
              )}
            </p>
            <div className="flex shrink-0 items-center gap-1.5">
              {shown.status === "ready" && (
                <>
                  {shown.notes_url && (
                    <Button size="xs" variant="ghost" iconRight={<ArrowUpRight />} onClick={() => desktop?.openExternal(shown.notes_url)}>
                      {t("desktop.updateNotes")}
                    </Button>
                  )}
                  <Button
                    size="xs"
                    variant="primary"
                    loading={restarting}
                    onClick={() => {
                      setRestarting(true);
                      desktop?.update?.restart();
                      // The app quits now; if it can't, the button comes back.
                      setTimeout(() => setRestarting(false), 8000);
                    }}
                  >
                    {t("desktop.updateRestart")}
                  </Button>
                </>
              )}
              <Button size="xs" variant="ghost" square icon={<X />} aria-label={t("desktop.updateLater")} onClick={() => setDismissed(key)} />
            </div>
            {shown.status === "downloading" && (
              <div aria-hidden className="absolute inset-x-0 -bottom-px h-px overflow-hidden">
                <motion.div
                  className={cn("h-full origin-left", tone.blue.solid)}
                  initial={false}
                  animate={{ scaleX: shown.percent / 100 }}
                  transition={reduce ? { duration: 0 } : { duration: dur.slow, ease: ease.out }}
                />
              </div>
            )}
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
