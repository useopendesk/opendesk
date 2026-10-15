import { useInfiniteQuery } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";
import type { Ticket } from "@/lib/api/types";
import { q } from "@/lib/queries";
import { realtime } from "@/lib/realtime/manager";
import { useRealtime } from "@/lib/realtime/store";
import { useSession } from "@/lib/session";
import { useNow } from "@/lib/useNow";
import { chatWait, handlerOf, useAgentOn, type ChatWait } from "./waiting";

// Live chat visitors waiting with nobody on it, for the whole app: the sidebar, the
// alerts and the tab title read the same list, longest wait first. Presence on every
// waiting chat is followed, so one a teammate (or you) picked up drops out at once.

const QUERY = { status: ["open", "pending"], assignee: "none", channel: "chat" } as const;

export type UrgentChat = { ticket: Ticket; wait: ChatWait };

/** `watch`: follow presence on the waiting chats; only the app-wide alerts hook sets it. */
export function useUrgentChats(enabled = true, watch = false) {
  const agentOn = useAgentOn();
  const list = useInfiniteQuery({ ...q.tickets({ ...QUERY, status: [...QUERY.status] }), enabled });
  const tickets = useMemo(() => list.data?.pages.flatMap((p) => p.data) ?? [], [list.data]);
  const now = useNow(tickets.length > 0);
  const presence = useRealtime((s) => s.presence);
  const active = useRealtime((s) => s.active);
  const me = useSession((s) => s.user?.id);

  const waitingIds = useMemo(() => tickets.filter((x) => !x.assignee && x.channel === "chat").map((x) => x.id).join(","), [tickets]);
  useEffect(() => {
    if (!enabled || !watch) return;
    realtime()?.setWatched(waitingIds ? waitingIds.split(",") : [], "waiting");
    return () => realtime()?.setWatched([], "waiting");
  }, [waitingIds, enabled, watch]);

  const urgent = useMemo(
    () =>
      tickets
        .filter((x) => !x.assignee)
        .map((x) => ({ ticket: x, wait: chatWait(x, agentOn, now, handlerOf(x, presence[x.id], me, active)) }))
        .filter((x): x is UrgentChat => x.wait?.state === "urgent")
        .sort((a, b) => a.wait.since - b.wait.since),
    [tickets, agentOn, now, presence, me, active],
  );
  return { urgent, now, ready: list.isSuccess };
}
