import type { QueryClient } from "@tanstack/react-query";
import { isPlanLimit } from "@/lib/api/client";
import { qk } from "@/lib/queries";
import { toast } from "@/components/ui/toast";

/** Plan-limit errors open the shared upgrade dialog; aborts are silent; anything else toasts. */
export function reportAiError(e: unknown, fallback: string) {
  if (isPlanLimit(e)) return;
  if (e instanceof DOMException && e.name === "AbortError") return;
  if ((e as { name?: string } | null)?.name === "AbortError") return;
  toast.error(e, fallback);
}

export function isAbort(e: unknown) {
  return (e as { name?: string } | null)?.name === "AbortError";
}

/** The AI balance changed: refresh the billing meter wherever it is shown. */
export function balanceSpent(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: qk.billing, exact: true });
}
