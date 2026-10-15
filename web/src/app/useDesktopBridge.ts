// The desktop app's side of the page (contracts/native.md, "Desktop shell"):
// deep links and notification clicks navigate, the dock or taskbar badge follows
// the unread count, and notifications reach the OS while the window is in the
// background, when the person's preferences let them (the type's desktop channel
// is on and notifications are not paused). All of it is a no-op in a browser.

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { useEffect } from "react";
import { safeNext } from "@/components/auth/parts";
import { i18n } from "@/i18n";
import type { Notification, NotificationPreferences } from "@/lib/api/types";
import { desktop, isDesktop } from "@/lib/desktop";
import { wantsDesktop } from "@/lib/notificationPrefs";
import { q, qk } from "@/lib/queries";
import { queryClient } from "@/lib/queryClient";
import { describe, notificationPath } from "./Notifications";

/** `opendesk://open?path=…` and notification clicks: always on, signed in or not. */
export function useDesktopNavigation() {
  const router = useRouter();
  useEffect(() => {
    if (!desktop) return;
    return desktop.onNavigate((path) => {
      const p = safeNext(path);
      if (p) router.history.push(p);
    });
  }, [router]);
}

/** Keeps the badge equal to the unread notification count from the query cache. */
export function useDesktopBadge() {
  const { data } = useQuery({ ...q.notifications(), enabled: isDesktop });
  const unread = data?.filter((n) => !n.read_at).length;
  useEffect(() => {
    if (desktop && unread !== undefined) desktop.setBadge(unread);
  }, [unread]);
  useEffect(() => () => desktop?.setBadge(0), []);
}

/** Keeps the notification preferences loaded in the desktop app, so notifyDesktop can follow them. */
export function useDesktopPreferences() {
  useQuery({ ...q.notificationPreferences(), enabled: isDesktop, staleTime: 5 * 60_000 });
}

/**
 * Mirrors a new notification to the OS when the window isn't focused (the toasts
 * cover the focused window), its type's desktop channel is on and nothing is
 * paused. Preferences not loaded yet follow the defaults: on for every type.
 */
export function notifyDesktop(n: Notification) {
  if (!desktop || document.hasFocus()) return;
  if (!wantsDesktop(queryClient.getQueryData<NotificationPreferences>(qk.notificationPreferences), n.type)) return;
  const t = i18n.getFixedT(null, "shell");
  const body = n.ticket ? `#${n.ticket.number} ${n.ticket.subject ?? ""}`.trim() : undefined;
  desktop.notify({ id: n.id, title: describe(n, t), body, path: notificationPath(n) ?? undefined });
}
