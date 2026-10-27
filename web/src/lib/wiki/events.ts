// wiki_page.* events (contracts/wiki.md "Events") on the TanStack Query cache. They carry
// the page without its body: the tree is patched in place, and an open page someone else
// changed is refetched (the page view then takes it, or asks when typing is unsaved).

import type { QueryClient } from "@tanstack/react-query";
import type { UserRef, WikiNode, WikiPage } from "../api/types";
import { qk } from "../queries";
import { removeSubtree, upsertNode } from "./tree";

export function applyWikiEvent(qc: QueryClient, verb: string, d: Record<string, unknown>, me: string) {
  void qc.invalidateQueries({ queryKey: qk.of("wiki", "search") });
  const patch = (fn: (nodes: WikiNode[]) => WikiNode[]) => qc.setQueryData<WikiNode[]>(qk.wikiTree, (old) => (old ? fn(old) : old));
  if (verb === "deleted") {
    const ids = (d.wiki_page_ids as string[] | undefined) ?? [d.wiki_page_id as string];
    patch((nodes) => removeSubtree(nodes, ids));
    for (const id of ids) qc.removeQueries({ queryKey: qk.wikiPage(id) });
    return;
  }
  const node = d.wiki_page as WikiNode | undefined;
  if (!node) return;
  if (verb === "moved") {
    // Positions of its old and new siblings moved too, and breadcrumbs below it.
    void qc.invalidateQueries({ queryKey: qk.wikiTree });
    void qc.invalidateQueries({ queryKey: qk.of("wiki", "page") });
    return;
  }
  patch((nodes) => upsertNode(nodes, node));
  const by = d.updated_by as UserRef | null | undefined;
  const page = qc.getQueryData<WikiPage>(qk.wikiPage(node.id));
  // The person's own saves come back with their answer; the event adds nothing.
  if (page && page.version < node.version && by?.id !== me) void qc.invalidateQueries({ queryKey: qk.wikiPage(node.id) });
  void qc.invalidateQueries({ queryKey: qk.wikiRevisions(node.id), refetchType: "none" });
}
