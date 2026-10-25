// Spaces hooks: the list (saved optimistically, put back if refused), starter Spaces, the
// data each workspace's rows read (the open one from the app's own caches, the others with
// OD-Workspace set to them under ["ws", wid]), and the watcher that keeps the others live.

import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { api, rows } from "../api/endpoints";
import type { MailAccount, MailCounts, MailFolder, Role, SidebarPrefs, Space, SpacesResponse, Team, TicketCounts, View } from "../api/types";
import { useAllFolders, useCollab, useMailAccounts, useSidebarPrefs } from "../collab/hooks";
import { normalizePrefs } from "../collab/sidebarPrefs";
import { q, qk } from "../queries";
import { useRole, useSession } from "../session";
import { teamsOf } from "../ticketCache";
import { forWire, normalizeSpace, normalizeSpaces } from "./model";
import { pollInterval, SpaceWatcher, watchList } from "./watch";

// ─── A memory of the last answers ──────────────────────────────────────────
// A workspace switch clears the whole query cache. What the sidebar last showed stays here
// as placeholder data, so the Space bar and other workspaces' rows hold still meanwhile.

const memo = new Map<string, unknown>();
const memoKey = (k: readonly unknown[]) => JSON.stringify(k);
function remember<T>(k: readonly unknown[], v: T): T {
  if (v !== undefined) memo.set(memoKey(k), v);
  return v;
}
function recall<T>(k: readonly unknown[]): T | undefined {
  return memo.get(memoKey(k)) as T | undefined;
}

// ─── The list ──────────────────────────────────────────────────────────────

/**
 * The user's Spaces. `fresh`: read from the API in this page load (not the copy kept across a
 * workspace switch), so a remembered id that names nothing can be let go.
 */
export function useSpaces(): { spaces: Space[] | undefined; fresh: boolean; isError: boolean } {
  const uid = useSession((s) => s.user?.id ?? "");
  const { data, isError, isPlaceholderData, isSuccess } = useQuery({ ...q.spaces(), placeholderData: () => recall<SpacesResponse>(["spaces", uid]) });
  useEffect(() => {
    if (data && !isPlaceholderData) remember(["spaces", uid], data);
  }, [data, uid, isPlaceholderData]);
  return { spaces: data?.spaces, fresh: isSuccess && !isPlaceholderData, isError };
}

// Saves go out one at a time, and only the newest list: quick changes coalesce, an older
// save never lands after a newer one, and only the newest answer is applied.
let saveSeq = 0;
let inFlight: Promise<SpacesResponse> | null = null;
let queued: Space[] | null = null;

function putSpaces(list: Space[]): Promise<SpacesResponse> {
  queued = list;
  const run = async (): Promise<SpacesResponse> => {
    let last: SpacesResponse = { spaces: list, saved: true };
    while (queued) {
      const next = queued;
      queued = null;
      const r = await api.me.spaces.put(next.map(forWire));
      last = { spaces: normalizeSpaces(r?.spaces), saved: true };
    }
    return last;
  };
  inFlight = (inFlight ?? Promise.resolve(null)).catch(() => null).then(run);
  return inFlight;
}

/** Saves the whole list: shown at once, put back if the API refuses it. */
export function useSaveSpaces() {
  const qc = useQueryClient();
  const uid = useSession((s) => s.user?.id ?? "");
  return useMutation({
    mutationFn: (list: Space[]) => putSpaces(list),
    onMutate: async (list) => {
      await qc.cancelQueries({ queryKey: qk.spaces });
      const prev = qc.getQueryData<SpacesResponse>(qk.spaces) ?? recall<SpacesResponse>(["spaces", uid]);
      const next = { spaces: list, saved: true };
      qc.setQueryData(qk.spaces, next);
      remember(["spaces", uid], next);
      return { prev, seq: ++saveSeq };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev && ctx.seq === saveSeq) {
        qc.setQueryData(qk.spaces, ctx.prev);
        remember(["spaces", uid], ctx.prev);
      }
    },
    onSuccess: (r, _v, ctx) => {
      if (ctx?.seq !== saveSeq) return;
      qc.setQueryData(qk.spaces, r);
      remember(["spaces", uid], r);
    },
  });
}

// ─── Starter Spaces ────────────────────────────────────────────────────────

/** A workspace's starter Space (null: none; undefined: loading). */
export function useStarterSpace(wid: string | null) {
  return useQuery({ ...q.starter(wid ?? ""), enabled: !!wid });
}

/** The starter Spaces of the given workspaces, by workspace id, once `enabled`. */
export function useStarterSpaces(wids: string[], enabled: boolean): Map<string, Space> {
  const results = useQueries({ queries: wids.map((wid) => ({ ...q.starter(wid), enabled, retry: false })) });
  const out = new Map<string, Space>();
  wids.forEach((wid, i) => {
    const s = results[i]?.data;
    if (s) out.set(wid, s);
  });
  return out;
}

/** Publishes the open workspace's starter Space (admins). Every item becomes the workspace's. */
export function useSaveStarter() {
  const qc = useQueryClient();
  const wid = useSession((s) => s.workspaceId);
  return useMutation({
    mutationFn: async (s: Space) => normalizeSpace((await api.starterSpace.put(forWire(s)))?.space),
    onSuccess: (s) => {
      if (wid) qc.setQueryData(qk.wsStarter(wid), s);
    },
  });
}

export function useRemoveStarter() {
  const qc = useQueryClient();
  const wid = useSession((s) => s.workspaceId);
  return useMutation({
    mutationFn: () => api.starterSpace.remove(),
    onSuccess: () => {
      if (wid) qc.setQueryData(qk.wsStarter(wid), null);
    },
  });
}

// ─── What each workspace's rows read ───────────────────────────────────────

/** Everything a workspace's rows in a Space read: the same shape for the open workspace and the others. */
export type WorkspaceData = {
  id: string;
  name: string;
  /** The open workspace (its rows are the sidebar's own, with menus and live counts). */
  open: boolean;
  role: Role;
  /** Whether it has Collab; undefined while unknown. */
  collab: boolean | undefined;
  /** The member's SidebarPrefs there (defaults without Collab). */
  prefs: SidebarPrefs;
  counts: TicketCounts | undefined;
  teams: Team[] | undefined;
  myTeams: ReadonlySet<string>;
  views: View[] | undefined;
  accounts: MailAccount[] | undefined;
  mailCounts: MailCounts | undefined;
  folders: Record<string, MailFolder[] | undefined>;
};

export const canSeeTeam = (d: Pick<WorkspaceData, "myTeams">, t: Team) => !t.private || d.myTeams.has(t.id);

const NO_FOLDERS: Record<string, MailFolder[] | undefined> = {};

/** The open workspace's data, from the caches the sidebar already reads; remembered for when it is no longer open. */
export function useOpenWorkspaceData(): WorkspaceData | null {
  const wid = useSession((s) => s.workspaceId);
  const uid = useSession((s) => s.user?.id ?? "");
  const name = useSession((s) => s.memberships.find((m) => m.workspace.id === s.workspaceId)?.workspace.name ?? "");
  const role = useRole();
  const collab = useCollab();
  const prefs = useSidebarPrefs(collab.enabled);
  const { data: counts } = useQuery(q.counts());
  const { data: teams } = useQuery(q.teams());
  const { data: views } = useQuery(q.views());
  const { data: accounts } = useMailAccounts();
  const { data: mailCounts } = useQuery({ ...q.mailCounts(), enabled: collab.enabled });
  const folders = useAllFolders(collab.enabled ? accounts : undefined);
  // When this workspace is no longer the open one, its rows read these until its own answers come.
  useEffect(() => {
    if (!wid) return;
    if (collab.status) remember(qk.wsCollab(wid), collab.status);
    if (counts) remember(qk.wsCounts(wid), counts);
    if (teams) remember(qk.wsTeams(wid), teams);
    if (views) remember(qk.wsViews(wid), views);
    if (accounts) remember(qk.wsMailAccounts(wid), accounts);
    if (mailCounts) remember(qk.wsMailCounts(wid), mailCounts);
    if (collab.enabled) remember(qk.wsSidebar(wid), prefs);
  }, [wid, collab.status, collab.enabled, counts, teams, views, accounts, mailCounts, prefs]);
  return useMemo(() => {
    if (!wid) return null;
    return {
      id: wid,
      name,
      open: true,
      role,
      collab: collab.isFetched ? collab.enabled : undefined,
      prefs,
      counts,
      teams,
      myTeams: new Set(teamsOf(teams ?? [], uid)),
      views,
      accounts: collab.enabled ? accounts : undefined,
      mailCounts: collab.enabled ? mailCounts : undefined,
      folders: collab.enabled ? folders : NO_FOLDERS,
    };
  }, [wid, name, role, collab.isFetched, collab.enabled, prefs, counts, teams, uid, views, accounts, mailCounts, folders]);
}

/**
 * A workspace switch clears the whole query cache. The newly open workspace's sidebar data starts
 * from what was last read of it as another workspace (marked stale, so it is read again at once),
 * so its rows and numbers hold still instead of blinking out.
 */
export function useCarryOver() {
  const qc = useQueryClient();
  const wid = useSession((s) => s.workspaceId);
  useLayoutEffect(() => {
    if (!wid) return;
    const seed = (key: readonly unknown[], from: readonly unknown[]) => {
      if (qc.getQueryData(key) !== undefined) return;
      const v = recall(from);
      if (v !== undefined) qc.setQueryData(key, v, { updatedAt: 1 });
    };
    seed(qk.collabStatus, qk.wsCollab(wid));
    seed(qk.counts, qk.wsCounts(wid));
    seed(qk.teams, qk.wsTeams(wid));
    seed(qk.views, qk.wsViews(wid));
    seed(qk.mailAccounts, qk.wsMailAccounts(wid));
    seed(qk.mailCounts, qk.wsMailCounts(wid));
    seed(qk.mailSidebar, qk.wsSidebar(wid));
  }, [qc, wid]);
}

const remote = <T,>(key: readonly unknown[], fn: () => Promise<T>, wid: string, enabled: boolean, staleTime = 30_000) => ({
  queryKey: key,
  queryFn: async () => remember(key, await fn()),
  enabled,
  staleTime,
  retry: false,
  refetchOnWindowFocus: true,
  refetchInterval: () => pollInterval(wid),
  placeholderData: () => recall<T>(key),
});

/**
 * Other workspaces' data, read with OD-Workspace set to each (never the open workspace's
 * caches). `folders`: the workspaces whose accounts' folders are needed (folder rows).
 */
export function useWorkspacesData(wids: string[], opts: { folders?: (wid: string) => boolean; enabled?: boolean } = {}): Map<string, WorkspaceData> {
  const uid = useSession((s) => s.user?.id ?? "");
  const memberships = useSession((s) => s.memberships);
  const on = opts.enabled !== false;
  const status = useQueries({ queries: wids.map((w) => remote(qk.wsCollab(w), () => api.inWorkspace(w).collabStatus().catch(() => null), w, on, 5 * 60_000)) });
  const counts = useQueries({ queries: wids.map((w) => remote(qk.wsCounts(w), () => api.inWorkspace(w).counts(), w, on)) });
  const teams = useQueries({ queries: wids.map((w) => remote(qk.wsTeams(w), async () => rows(await api.inWorkspace(w).teams()), w, on, 60_000)) });
  const views = useQueries({ queries: wids.map((w) => remote(qk.wsViews(w), async () => rows(await api.inWorkspace(w).views()), w, on, 60_000)) });
  const collabOf = (i: number) => status[i]?.data?.enabled === true;
  const accounts = useQueries({ queries: wids.map((w, i) => remote(qk.wsMailAccounts(w), async () => rows(await api.inWorkspace(w).mailAccounts()), w, on && collabOf(i))) });
  const mailCounts = useQueries({ queries: wids.map((w, i) => remote(qk.wsMailCounts(w), () => api.inWorkspace(w).mailCounts(), w, on && collabOf(i))) });
  const sidebar = useQueries({ queries: wids.map((w, i) => remote(qk.wsSidebar(w), async () => normalizePrefs(await api.inWorkspace(w).sidebar()), w, on && collabOf(i), 5 * 60_000)) });
  const pairs = wids.flatMap((w, i) => (collabOf(i) && opts.folders?.(w) ? (accounts[i]?.data ?? []).map((a) => [w, a.id] as const) : []));
  const folders = useQueries({ queries: pairs.map(([w, a]) => remote(qk.wsMailFolders(w, a), async () => rows(await api.inWorkspace(w).mailFolders(a)), w, on, 60_000)) });
  const out = new Map<string, WorkspaceData>();
  wids.forEach((w, i) => {
    const m = memberships.find((x) => x.workspace.id === w);
    if (!m) return;
    const st = status[i];
    const collab = st?.data ? st.data.enabled === true : st?.isFetched ? false : undefined;
    const fs: Record<string, MailFolder[] | undefined> = {};
    pairs.forEach(([pw, a], j) => {
      if (pw === w) fs[a] = folders[j]?.data;
    });
    const tm = teams[i]?.data;
    out.set(w, {
      id: w,
      name: m.workspace.name,
      open: false,
      role: m.role,
      collab,
      prefs: collab ? normalizePrefs(sidebar[i]?.data) : normalizePrefs(null),
      counts: counts[i]?.data,
      teams: tm,
      myTeams: new Set(teamsOf(tm ?? [], uid)),
      views: views[i]?.data,
      accounts: collab ? accounts[i]?.data : undefined,
      mailCounts: collab ? mailCounts[i]?.data : undefined,
      folders: fs,
    });
  });
  return out;
}

/** The open workspace's data and the others', by id. */
export function useAllWorkspacesData(wids: string[], opts: { folders?: (wid: string) => boolean; enabled?: boolean } = {}): Map<string, WorkspaceData> {
  const open = useOpenWorkspaceData();
  const others = useMemo(() => wids.filter((w) => w !== open?.id), [wids, open?.id]);
  const data = useWorkspacesData(others, opts);
  if (open) data.set(open.id, open);
  return data;
}

/** Keeps the other workspaces in `wids` live while mounted; their sockets close when they leave the list. */
export function useSpaceWatch(wids: string[]) {
  const qc = useQueryClient();
  const open = useSession((s) => s.workspaceId);
  const uid = useSession((s) => s.user?.id);
  const ref = useRef<SpaceWatcher | null>(null);
  const list = watchList(wids, open).join(",");
  useEffect(() => {
    if (!uid) return;
    const w = new SpaceWatcher(qc);
    ref.current = w;
    return () => {
      w.stop();
      ref.current = null;
    };
  }, [qc, uid]);
  useEffect(() => {
    ref.current?.sync(list ? list.split(",") : []);
  }, [list, uid]);
}
