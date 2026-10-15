// AI availability and costs from GET /v1/ai/config. Every AI control renders
// only when `useAI().enabled`; costs come from the server, never hard-coded.

import { useQuery } from "@tanstack/react-query";
import type { AIConfig, AIFeature, AISettings } from "./api/types";
import { q } from "./queries";

export type UseAI = {
  /** The instance has a provider and the workspace has AI switched on. */
  enabled: boolean;
  config: AIConfig | null;
  features: AISettings | null;
  /** What a typical call of `feature` costs in US dollars, or null when unknown. */
  cost: (feature: AIFeature) => number | null;
};

export function useAI(): UseAI {
  const { data } = useQuery(q.aiConfig());
  const config = data ?? null;
  return {
    enabled: !!config?.enabled && config.features?.enabled !== false,
    config,
    features: config?.features ?? null,
    cost: (f) => config?.costs?.[f] ?? null,
  };
}
