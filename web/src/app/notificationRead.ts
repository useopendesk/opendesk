// Marking notifications read, optimistically in the cache the bell reads. Opening
// a ticket marks its notifications read, so nobody is emailed about a ticket they
// are already looking at.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import { api } from "@/lib/api/endpoints";
import type { Notification } from "@/lib/api/types";
import { q, qk } from "@/lib/queries";

export type ReadBody = { ids?: string[]; all?: boolean };

/** The list with the given notifications (or all of them) read; read ones keep their time. */
export function withRead(list: Notification[], b: ReadBody, now: string): Notification[] {
  const ids = new Set(b.ids ?? []);
  return list.map((n) => (!n.read_at && (b.all || ids.has(n.id)) ? { ...n, read_at: now } : n));
}

/** The unread notifications about one ticket. */
export function unreadForTicket(list: Notification[] | undefined, ticketId: string): string[] {
  if (!list || !ticketId) return [];
  return list.filter((n) => !n.read_at && n.ticket?.id === ticketId).map((n) => n.id);
}

/** POST /v1/notifications/read, applied to the cache first and rolled back on failure. */
export function useMarkNotificationsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (b: ReadBody) => api.notifications.read(b),
    onMutate: async (b) => {
      await qc.cancelQueries({ queryKey: qk.notifications });
      const prev = qc.getQueryData<Notification[]>(qk.notifications);
      if (prev) qc.setQueryData<Notification[]>(qk.notifications, withRead(prev, b, new Date().toISOString()));
      return { prev };
    },
    onError: (_e, _b, ctx) => {
      if (ctx?.prev) qc.setQueryData(qk.notifications, ctx.prev);
    },
  });
}

function subscribeVisibility(cb: () => void) {
  document.addEventListener("visibilitychange", cb);
  return () => document.removeEventListener("visibilitychange", cb);
}

/**
 * While a ticket is open in a visible tab, its unread notifications are marked
 * read: the ones there when it opens and any that arrive while it stays open.
 * Each id is tried once per open, so a failed request never loops.
 */
export function useMarkTicketNotificationsRead(ticketId: string) {
  const { data } = useQuery(q.notifications());
  const visible = useSyncExternalStore(subscribeVisibility, () => document.visibilityState !== "hidden", () => true);
  const markRead = useMarkNotificationsRead();
  const tried = useRef(new Set<string>());
  const ids = useMemo(() => unreadForTicket(data, ticketId), [data, ticketId]);
  const key = ids.join(",");

  useEffect(() => {
    tried.current = new Set();
  }, [ticketId]);

  useEffect(() => {
    if (!visible) return;
    const todo = ids.filter((id) => !tried.current.has(id));
    if (!todo.length) return;
    todo.forEach((id) => tried.current.add(id));
    markRead.mutate({ ids: todo });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` stands for `ids`
  }, [key, visible]);
}
