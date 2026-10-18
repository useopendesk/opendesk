// The signed-in admin. The API client owns the token; this store mirrors who
// is signed in so the router and the shell can follow it.

import { useSyncExternalStore } from "react";
import { onSession, refreshSession } from "./api/client";
import type { AdminMe } from "./api/types";

export type SessionState =
  | { status: "loading"; me: null }
  | { status: "authed"; me: AdminMe }
  /** `ended`: why a session ended on its own (a code), or "logout" when the admin signed out. */
  | { status: "anon"; me: null; ended?: string };

let state: SessionState = { status: "loading", me: null };
const listeners = new Set<() => void>();

function set(next: SessionState) {
  state = next;
  listeners.forEach((l) => l());
}

let wasAuthed = false;
function follow(r: Parameters<Parameters<typeof onSession>[0]>[0], reason?: Parameters<Parameters<typeof onSession>[0]>[1]) {
  if (r) {
    wasAuthed = true;
    set({ status: "authed", me: r.admin });
    return;
  }
  // An ended session the admin did not end themselves carries a reason; a first
  // visit without a session carries none.
  const ended = wasAuthed ? (reason ? reason.code : "logout") : undefined;
  wasAuthed = false;
  set({ status: "anon", me: null, ended });
}
let unfollow = onSession(follow);

export function getSession() {
  return state;
}

export function subscribeSession(l: () => void) {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

let booting: Promise<SessionState> | null = null;

/** Resolves the session once: on load the refresh cookie is tried; a 401 means sign in. */
export function bootSession(): Promise<SessionState> {
  if (state.status !== "loading") return Promise.resolve(state);
  booting ??= refreshSession()
    .then(() => state)
    .catch((e) => {
      // The API could not answer: keep "loading" so the next navigation tries again.
      booting = null;
      throw e;
    });
  return booting;
}

/** Update the admin's own record (GET /v1/admin/me) without a new token. */
export function updateMe(me: AdminMe) {
  if (state.status === "authed") set({ status: "authed", me });
}

export function useSession<T>(select: (s: SessionState) => T): T {
  return useSyncExternalStore(subscribeSession, () => select(state));
}

/** The signed-in admin; only inside the shell, which renders once there is one. */
export function useMe(): AdminMe {
  const me = useSession((s) => s.me);
  if (!me) throw new Error("useMe outside a signed-in shell");
  return me;
}

/** Test hook. */
export function __resetSessionForTests() {
  unfollow();
  unfollow = onSession(follow);
  state = { status: "loading", me: null };
  booting = null;
  wasAuthed = false;
}
