import { describe, expect, it } from "vitest";
import { dropLive, liveMessage, reduceLive, repliesFor, type LiveState } from "./live";
import type { LiveEvent } from "./types";

const empty = (): LiveState => ({ replies: {}, closed: [] });
const R = "11111111-1111-4111-8111-111111111111";
const T = "22222222-2222-4222-8222-222222222222";
const started: LiveEvent = { type: "ai.reply.started", data: { reply_id: R, ticket_id: T, bot: { name: "Ollie" } } };
const delta = (text: string): LiveEvent => ({ type: "ai.reply.delta", data: { reply_id: R, ticket_id: T, text } });
const tool = (status: "running" | "done" | "error", name = "Order lookup"): LiveEvent => ({ type: "ai.reply.tool", data: { reply_id: R, ticket_id: T, tool: name, status } });
const ended = (message_id: string | null, handoff = false): LiveEvent => ({ type: "ai.reply.ended", data: { reply_id: R, ticket_id: T, message_id, handoff, action_id: null } });
const none = () => false;

function run(events: LiveEvent[], posted: (id: string) => boolean = none, st = empty()) {
  return events.reduce((s, e, i) => reduceLive(s, e, i, posted), st);
}

describe("live replies", () => {
  it("goes typing, streaming, done", () => {
    let st = run([started]);
    expect(st.replies[R]).toMatchObject({ phase: "typing", bot: { name: "Ollie", look: null }, partial: false, text: "" });
    st = run([delta("Hello "), delta("there [1].")], none, st);
    expect(st.replies[R]).toMatchObject({ phase: "streaming", text: "Hello there [1]." });
    st = run([ended("m1")], none, st);
    expect(st.replies[R]).toMatchObject({ phase: "done", message_id: "m1" });
    expect(repliesFor(st, T)).toHaveLength(1);
    expect(repliesFor(st, "other")).toHaveLength(0);
  });

  it("keeps the bot a reply is written as, with its look read leniently", () => {
    const ev: LiveEvent = {
      type: "ai.reply.started",
      data: { reply_id: R, ticket_id: T, bot: { id: "b1", name: "Pip", avatar_url: "https://cdn.test/pip.png", look: { kind: "built", shape: "cloud", eyes: "<x>" } as never } },
    };
    const r = run([ev]).replies[R];
    expect(r.bot).toMatchObject({ id: "b1", name: "Pip", avatar_url: "https://cdn.test/pip.png", look: { kind: "built", shape: "cloud", eyes: "pills" } });
    // A later started without a bot keeps it.
    expect(run([{ type: "ai.reply.started", data: { reply_id: R, ticket_id: T } }], none, run([ev])).replies[R].bot?.name).toBe("Pip");
  });

  it("tracks tool calls and settles running ones on end", () => {
    let st = run([started, tool("running"), tool("done"), tool("running", "Orders")]);
    expect(st.replies[R].tools).toEqual([
      { name: "Order lookup", status: "done" },
      { name: "Orders", status: "running" },
    ]);
    st = run([ended(null)], none, st);
    expect(st.replies[R].tools[1].status).toBe("done");
  });

  it("swaps to the posted message when it arrives after ended", () => {
    const st = run([started, delta("Hi"), ended("m1")]);
    const { state, replaced } = liveMessage(st, { id: "m1", ticket_id: T, ai: true, direction: "outbound" });
    expect(replaced).toBe(true);
    expect(state.replies[R]).toBeUndefined();
    expect(state.closed).toContain(R);
  });

  it("drops the preview when the message came first", () => {
    // message.created before ended: the writing preview on that ticket goes...
    let st = run([started, delta("Hi")]);
    const r = liveMessage(st, { id: "m1", ticket_id: T, ai: true, direction: "outbound" });
    expect(r.replaced).toBe(true);
    expect(r.state.replies[R]).toBeUndefined();
    // ...and a late ended or delta can't bring it back.
    st = run([delta(" again"), ended("m1")], none, r.state);
    expect(st.replies[R]).toBeUndefined();
    // ended naming a message already in the thread just closes.
    st = run([started, delta("x"), ended("m9")], (id) => id === "m9");
    expect(st.replies[R]).toBeUndefined();
  });

  it("does not let a team message or another ticket replace the preview", () => {
    const st = run([started, delta("Hi")]);
    expect(liveMessage(st, { id: "h", ticket_id: T, ai: false, direction: "outbound" }).replaced).toBe(false);
    expect(liveMessage(st, { id: "m", ticket_id: "other", ai: true, direction: "outbound" }).replaced).toBe(false);
  });

  it("fades out when nothing was posted", () => {
    let st = run([started, delta("Hm"), ended(null, true)]);
    expect(st.replies[R]).toMatchObject({ phase: "leaving", handoff: true });
    st = dropLive(st, { id: R });
    expect(st.replies[R]).toBeUndefined();
  });

  it("joins mid-reply as partial and ignores events after end", () => {
    let st = run([delta("…middle of a sentence")]);
    expect(st.replies[R]).toMatchObject({ partial: true, phase: "streaming" });
    st = run([ended("m1"), delta("late")], none, st);
    expect(st.replies[R].text).toBe("…middle of a sentence");
  });

  it("ignores malformed events", () => {
    const bad = { type: "ai.reply.delta", data: { text: "x" } } as unknown as LiveEvent;
    expect(run([bad])).toEqual(empty());
    expect(run([delta("")]).replies[R]).toBeUndefined();
  });

  it("drops replies that went quiet", () => {
    const st = run([started]); // at = 0
    expect(dropLive(st, { idleBefore: 1, doneBefore: 1 }).replies[R]).toBeUndefined();
    expect(dropLive(st, { idleBefore: 0, doneBefore: 0 }).replies[R]).toBeDefined();
  });
});
