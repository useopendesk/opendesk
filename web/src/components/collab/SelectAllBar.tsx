// The master checkbox above Collab's conversation list, with the scope of what it selects:
// the conversations loaded, or (one click more) every one in the folder in view.

import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";

/** The most conversations "select all in the folder" loads and selects at once. */
export const SELECT_ALL_MAX = 1000;

export type SelectAllBarProps = {
  /** Conversations loaded into the list. */
  loaded: number;
  selected: number;
  /** More conversations than the loaded ones exist in this box or folder. */
  hasMore: boolean;
  /** Loading the rest, to select it. */
  loading: boolean;
  /** The box or folder in view, by its name. */
  scope: string;
  /** Selects, or clears, the loaded conversations. */
  onToggle: () => void;
  /** Loads the rest of the folder and selects it. */
  onSelectEvery: () => void;
};

export function SelectAllBar({ loaded, selected, hasMore, loading, scope, onToggle, onSelectEvery }: SelectAllBarProps) {
  const { t } = useTranslation("collab");
  const all = loaded > 0 && selected === loaded;
  const state = all ? true : selected > 0 ? "indeterminate" : false;
  // The note under the checkbox says what "all" means, never silently more than it is.
  const note = loading
    ? t("list.selecting")
    : !all
    ? null
    : hasMore && loaded >= SELECT_ALL_MAX
      ? t("list.firstSelected", { count: loaded, scope })
      : hasMore
        ? t("list.pageSelected", { count: loaded })
        : loaded > 50
          ? t("list.folderSelected", { count: loaded, scope })
          : null;
  return (
    <div className="flex min-h-9 shrink-0 items-center gap-2.5 border-b border-line-soft px-3.5 text-[12px] text-fg-3">
      <Checkbox checked={state} disabled={!loaded || loading} aria-label={selected ? t("list.clear") : t("list.selectAll")} onCheckedChange={onToggle} />
      <span className="mono tabular-nums">{selected ? t("list.selected", { count: selected }) : t("list.selectAll")}</span>
      {note && (
        <span className="flex min-w-0 items-center gap-1.5 truncate" role="status">
          <span className="truncate">{note}</span>
          {hasMore && !loading && loaded < SELECT_ALL_MAX && (
            <Button size="sm" variant="ghost" onClick={onSelectEvery}>
              {t("list.selectEvery", { scope })}
            </Button>
          )}
        </span>
      )}
    </div>
  );
}
