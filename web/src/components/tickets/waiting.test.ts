import { describe, expect, it } from "vitest";
import { ticket } from "@/lib/test/fixtures";
import { chatWait, formatWait, handlerOf } from "./waiting";

const now = Date.parse("2026-10-12T10:00:00Z");
const ago = (s: number) => new Date(now - s * 1000).toISOString();

describe("chatWait", () => {
  it("is urgent when a visitor waits and the agent is off", () => {
    const w = chatWait(ticket({ channel: "chat", waiting_since: ago(30) }), false, now);
    expect(w).toMatchObject({ state: "urgent", since: now - 30_000 });
  });

  it("is calm while Ollie answers, then urgent after a minute", () => {
    const t = ticket({ channel: "chat", waiting_since: ago(20) });
    expect(chatWait(t, true, now)?.state).toBe("ollie");
    expect(chatWait(t, true, now + 45_000)?.state).toBe("urgent");
  });

  it("counts from the handoff when Ollie's handoff line was the last reply", () => {
    const t = ticket({ channel: "chat", waiting_since: null, custom: { ai_handoff: true, ai_handoff_at: ago(90) } });
    expect(chatWait(t, true, now)).toMatchObject({ state: "urgent", since: now - 90_000, handedOff: true });
    // A person replied after the handoff: nobody is waiting.
    expect(chatWait({ ...t, first_responded_at: ago(10) }, true, now)).toBeNull();
  });

  it("ignores email, spam and closed conversations", () => {
    expect(chatWait(ticket({ channel: "email", waiting_since: ago(30) }), false, now)).toBeNull();
    expect(chatWait(ticket({ channel: "chat", spam: true, waiting_since: ago(30) }), false, now)).toBeNull();
    expect(chatWait(ticket({ channel: "chat", status: "solved", waiting_since: ago(30) }), false, now)).toBeNull();
  });
});

describe("handled chats", () => {
  const t = ticket({ channel: "chat", waiting_since: ago(120) });
  const presence = (state: "viewing" | "replying" | "idle", user_id = "u2") => ({ [user_id]: { metas: [{ user_id, name: "Anna", state }] } });

  it("is handled while you have it open", () => {
    expect(chatWait(t, false, now, handlerOf(t, undefined, "u1", t.id))).toMatchObject({ state: "handled", handler: { me: true, how: "open" } });
  });

  it("is handled while a teammate views or replies, not while they idle", () => {
    expect(handlerOf(t, presence("replying"), "u1", null)).toEqual({ me: false, name: "Anna", how: "replying" });
    expect(handlerOf(t, presence("viewing"), "u1", null)?.how).toBe("viewing");
    expect(handlerOf(t, presence("idle"), "u1", null)).toBeNull();
    expect(chatWait(t, false, now, handlerOf(t, presence("idle"), "u1", null))?.state).toBe("urgent");
  });

  it("never counts yourself from presence, only from the open ticket", () => {
    expect(handlerOf(t, presence("replying", "u1"), "u1", null)).toBeNull();
  });

  it("is handled once assigned", () => {
    const mine = ticket({ ...t, assignee: { id: "u1", name: "Maya", email: "m@x.io", avatar_url: null } });
    expect(handlerOf(mine, undefined, "u1", null)).toEqual({ me: true, name: "Maya", how: "assigned" });
  });

  it("stays with Ollie while Ollie is answering", () => {
    const fresh = ticket({ channel: "chat", waiting_since: ago(10) });
    expect(chatWait(fresh, true, now, handlerOf(fresh, presence("viewing"), "u1", null))?.state).toBe("ollie");
  });
});

describe("formatWait", () => {
  it("reads at a glance", () => {
    expect(formatWait(45_000)).toBe("45s");
    expect(formatWait(192_000)).toBe("3m 12s");
    expect(formatWait(3_840_000)).toBe("1h 04m");
    expect(formatWait(3 * 86_400_000 + 2 * 3_600_000)).toBe("3d 2h");
  });
});
