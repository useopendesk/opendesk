import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { alertSoundOn, playAlertOnce } from "@/lib/alertSound";
import { useUrgentChats } from "./useUrgentChats";

// App-wide alerts for live chat visitors waiting with nobody on it, on every page: a
// chime when one starts waiting, a softer reminder every minute while any still waits,
// and a blinking tab title while the tab is in the background. A chat someone picked up
// (open, viewing, replying or assigned) stops alerting.

const REMINDER_MS = 60_000;

export function useWaitingAlerts(enabled: boolean) {
  const { t } = useTranslation("inbox");
  const { urgent, now, ready } = useUrgentChats(enabled, true);
  const key = urgent
    .map((x) => x.ticket.id)
    .sort()
    .join(",");

  const seen = useRef<Set<string> | null>(null);
  const lastSound = useRef(0);

  // A visitor started waiting: chime. The ones already waiting when the app opened are
  // covered by the reminder.
  useEffect(() => {
    if (!ready) return;
    const ids = key ? key.split(",") : [];
    if (!seen.current) {
      seen.current = new Set(ids);
      lastSound.current = Date.now();
      return;
    }
    const fresh = ids.filter((id) => !seen.current!.has(id));
    seen.current = new Set(ids);
    if (fresh.length) {
      playAlertOnce("waiting", fresh[0]);
      lastSound.current = Date.now();
    }
  }, [key, ready]);

  // Still waiting: a softer reminder every minute.
  useEffect(() => {
    if (!urgent.length || !alertSoundOn()) return;
    if (Date.now() - lastSound.current < REMINDER_MS) return;
    lastSound.current = Date.now();
    playAlertOnce("reminder", String(Math.floor(Date.now() / REMINDER_MS)));
  }, [now, urgent.length]);

  // In a background tab, the title blinks with the count.
  const count = urgent.length;
  useEffect(() => {
    if (!count) return;
    const alert = t("waiting.title", { count });
    let original = document.title;
    let showing = false;
    const tick = () => {
      if (!document.hidden) {
        if (showing) document.title = original;
        showing = false;
        return;
      }
      if (!showing) original = document.title;
      document.title = showing ? original : alert;
      showing = !showing;
    };
    const timer = setInterval(tick, 1200);
    return () => {
      clearInterval(timer);
      if (showing) document.title = original;
    };
  }, [count, t]);
}
