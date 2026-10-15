// The widget agent's settings (contracts/agent.md, contracts/ai.md), with the
// contract defaults for keys an older server leaves out, and the Behaviour
// page's unsaved draft, which the playground tries before it is saved.

import { create } from "zustand";
import type { AISettings, PlaygroundSettings } from "@/lib/api/types";

/** A bot's instructions (contracts/bots.md). */
export const INSTRUCTIONS_MAX = 4000;
export const TOPICS_MAX = 20;
export const TOPIC_MIN = 2;
export const TOPIC_LEN = 100;
export const CAP_MAX = 10_000;

export type AgentSettings = Required<
  Pick<
    AISettings,
    | "widget_answers"
    | "widget_model"
    | "widget_max_replies"
    | "widget_blocked_topics"
    | "widget_monthly_cap_usd"
    | "widget_collect_email"
  >
>;

export function agentSettings(f: AISettings | null | undefined): AgentSettings {
  return {
    widget_answers: f?.widget_answers ?? false,
    widget_model: f?.widget_model ?? "",
    widget_max_replies: f?.widget_max_replies ?? 20,
    widget_blocked_topics: f?.widget_blocked_topics ?? [],
    widget_monthly_cap_usd: f?.widget_monthly_cap_usd ?? 0,
    widget_collect_email: f?.widget_collect_email ?? true,
  };
}

/** Plain text a topic may hold: no markup or control characters. */
export function topicProblem(v: string): "short" | "long" | "markup" | null {
  const s = v.trim();
  if (s.length < TOPIC_MIN) return "short";
  if (s.length > TOPIC_LEN) return "long";
  // eslint-disable-next-line no-control-regex
  if (/[<>\u0000-\u001f\u007f]/.test(s)) return "markup";
  return null;
}

/** The Behaviour page's unsaved edits. */
export type BehaviourDraft = { ai: AgentSettings };

type DraftState = {
  /** Unsaved Behaviour edits, kept while you try them in the playground; null when there are none. */
  draft: BehaviourDraft | null;
  setDraft: (d: BehaviourDraft | null) => void;
};

export const useAgentDraft = create<DraftState>((set) => ({
  draft: null,
  setDraft: (draft) => set({ draft }),
}));

/** What the playground sends from a Behaviour draft. */
export function playgroundSettings(d: BehaviourDraft | null): PlaygroundSettings | undefined {
  if (!d) return undefined;
  return { widget_model: d.ai.widget_model, widget_blocked_topics: d.ai.widget_blocked_topics };
}
