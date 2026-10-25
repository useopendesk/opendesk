// The picked Space in the sidebar (contracts/spaces.md "Web app"): every row of it from every
// workspace, in its sections, each with an optional header that folds (saved in the Space). A
// whole built-in section of the open workspace is the sidebar's own section; one row of it is
// drawn by the shared row renderer with the Space's name and number; another workspace's rows
// read that workspace's numbers and open in it (`?ws=`), with the Space still picked.

import { ChevronRight, Plus } from "lucide-react";
import { Fragment, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Section, SectionMarkCtx, WorkspaceGlyph } from "@/app/sidebarParts";
import { useWaitingFor } from "@/components/tickets/useUrgentChats";
import type { SidebarSectionId, Space, SpaceItem, SpaceSection } from "@/lib/api/types";
import { hid, livePins } from "@/lib/collab/sidebarPrefs";
import { cn } from "@/lib/cn";
import { folders as ticketFolders } from "@/lib/folders";
import { useSession } from "@/lib/session";
import { canSeeTeam, useAllWorkspacesData, useSpaceWatch, type WorkspaceData } from "@/lib/spaces/hooks";
import { isSectionId, itemKey, otherWorkspaces, replaceSpace, showSpace, SPACE_PAGES, updateItem, updateSection } from "@/lib/spaces/model";
import { spaceUi } from "@/lib/spaces/store";
import { buildRow, NavItemRow, useMailLocation, useRowLabels, type RowContext, type RowModel } from "./rows";
import { RenameRow, SpaceRowMenu } from "./SpaceRowMenu";
import { useSpaceActions } from "./actions";

/** The open workspace's sections, in its order: the sidebar without Spaces. */
export function SectionList({ ids, render }: { ids: SidebarSectionId[]; render: (id: SidebarSectionId, first: boolean) => ReactNode }) {
  return (
    <div className="[&>*:first-child]:!mt-0">
      {ids.map((id, i) => (
        <Fragment key={id}>{render(id, i === 0)}</Fragment>
      ))}
    </div>
  );
}


function SpaceSectionBlock({ section, first, rail, onToggle, children }: { section: SpaceSection; first: boolean; rail: boolean; onToggle: () => void; children: ReactNode }) {
  const { t } = useTranslation("spaces");
  const title = section.title.trim();
  const folded = !!title && section.collapsed && !rail;
  return (
    <div className={cn(!first && "mt-5")}>
      {title && !rail && (
        <button
          type="button"
          aria-expanded={!folded}
          aria-label={folded ? t("view.expand", { name: title }) : t("view.collapse", { name: title })}
          onClick={onToggle}
          className="group/space mx-2.5 mb-1 flex h-7 w-[calc(100%-1.25rem)] items-center gap-1 rounded-md px-2 text-left text-[12px] font-semibold text-fg-2 outline-none transition-colors hover:bg-chrome-hover hover:text-fg focus-visible:shadow-[0_0_0_2px_var(--focus)]"
        >
          <span className="min-w-0 flex-1 truncate">{title}</span>
          <ChevronRight
            aria-hidden
            className={cn(
              "size-3.5 shrink-0 text-fg-4 transition-[transform,opacity] duration-150 motion-reduce:transition-none",
              folded ? "opacity-100" : "rotate-90 opacity-0 group-hover/space:opacity-100 group-focus-visible/space:opacity-100",
            )}
          />
        </button>
      )}
      {rail && !first && <div aria-hidden className="mx-4 mb-2 h-px bg-chrome-line" />}
      {!folded && <div className="animate-rise-in [&>*:first-child]:!mt-0 motion-reduce:animate-none">{children}</div>}
    </div>
  );
}

/** A whole built-in section of another workspace: that workspace's rows, by its own preferences. */
function RemoteSection({ d, id, first, rail, mark, ctx }: { d: WorkspaceData; id: SidebarSectionId; first: boolean; rail: boolean; mark: ReactNode; ctx: RowContext }) {
  const p = d.prefs;
  const hidden = new Set(p.hidden);
  const row = (kind: string, rid: string, via?: SidebarSectionId[]) => buildRow({ kind, id: rid }, d, { ...ctx, via });
  let list: (RowModel | null)[] = [];
  switch (id) {
    case "favorites":
      list = d.collab ? livePins(p, (pin) => row(pin.kind, pin.id, ["favorites"])).map((x) => x.target) : [];
      break;
    case "inbox":
      list = ticketFolders.filter((f) => !hidden.has(hid.ticketFolder(f.id))).map((f) => row("ticket_folder", f.id));
      break;
    case "mail": {
      if (!d.collab) break;
      const accounts = (d.accounts ?? []).filter((a) => !hidden.has(hid.account(a.id)));
      if (p.unified_inbox && (d.accounts?.length ?? 0) >= 2 && !hidden.has(hid.box("inbox"))) list.push(row("box", "inbox"));
      if (!hidden.has(hid.box("shared")) && d.mailCounts?.boxes.shared) list.push(row("box", "shared"));
      if (!hidden.has(hid.box("assigned")) && d.mailCounts?.boxes.assigned) list.push(row("box", "assigned"));
      list.push(...accounts.map((a) => row("account", a.id)));
      break;
    }
    case "teams": {
      const byName = (a: { name: string }, b: { name: string }) => a.name.localeCompare(b.name);
      const visible = (d.teams ?? []).filter((x) => canSeeTeam(d, x) && !hidden.has(hid.team(x.id)));
      const mine = visible.filter((x) => d.myTeams.has(x.id)).sort(byName);
      // The person's teams; someone on none of them gets the teams they can see.
      list = [...(mine.length > 1 ? [row("team", "mine")] : []), ...(mine.length ? mine : visible.sort(byName)).map((x) => row("team", x.id))];
      break;
    }
    case "views":
      list = [...(d.views ?? [])]
        .filter((v) => !hidden.has(hid.view(v.id)))
        .sort((a, b) => a.position - b.position)
        .map((v) => row("view", v.id));
      break;
    case "workspace":
      list = SPACE_PAGES.map((pg) => row("page", pg));
      break;
  }
  const models = list.filter((m): m is RowModel => !!m);
  if (!models.length) return null;
  return (
    <SectionMarkCtx.Provider value={mark}>
      <Section id={`ws-${d.id}-${id}`} label={ctx.labels.section(id)} rail={rail} first={first}>
        {models.map((m) => (
          <NavItemRow key={m.key} model={m} rail={rail} prefs={p} ws={d.id} />
        ))}
      </Section>
    </SectionMarkCtx.Provider>
  );
}

export function SpaceView({
  spaces,
  space,
  rail,
  renderSection,
  live = true,
}: {
  spaces: Space[];
  /** The picked Space. */
  space: Space;
  rail: boolean;
  /** Draws a built-in section of the open workspace. */
  renderSection: (id: SidebarSectionId, first: boolean) => ReactNode;
  /** Keeps other workspaces' numbers live (the phone's drawer leaves it to the sidebar that is always mounted). */
  live?: boolean;
}) {
  const { t } = useTranslation("spaces");
  const wid = useSession((s) => s.workspaceId);
  const memberships = useSession((s) => s.memberships);
  const members = useMemo(() => new Set(memberships.map((m) => m.workspace.id)), [memberships]);
  const actions = useSpaceActions(spaces);
  const wids = useMemo(() => {
    const out: string[] = [];
    for (const sec of space.sections) for (const i of sec.items) if (members.has(i.workspace_id) && !out.includes(i.workspace_id)) out.push(i.workspace_id);
    return out;
  }, [space, members]);
  const data = useAllWorkspacesData(wids, { folders: (w) => space.sections.some((sec) => sec.items.some((i) => i.workspace_id === w && i.kind === "folder")) });
  const shown = showSpace(space, { members, collab: (w) => data.get(w)?.collab });
  useSpaceWatch(live ? otherWorkspaces(shown, wid) : []);
  const labels = useRowLabels();
  const here = useMailLocation();
  const waitingFor = useWaitingFor();
  const [renaming, setRenaming] = useState<string | null>(null);

  const nameOf = (w: string) => memberships.find((m) => m.workspace.id === w)?.workspace.name ?? "";
  const markFor = (w: string): ReactNode =>
    shown.marks ? (
      <>
        <WorkspaceGlyph name={nameOf(w)} tiny />
        <span className="sr-only">{nameOf(w)}</span>
      </>
    ) : null;
  const change = (next: Space) => actions.put(replaceSpace(spaces, next));

  const renderItems = (section: SpaceSection, items: SpaceItem[]) => {
    // Runs of single rows sit together; a whole section brings its own header.
    const blocks: ({ kind: "section"; item: SpaceItem } | { kind: "rows"; items: SpaceItem[] })[] = [];
    for (const item of items) {
      const last = blocks[blocks.length - 1];
      if (item.kind === "section") blocks.push({ kind: "section", item });
      else if (last?.kind === "rows") last.items.push(item);
      else blocks.push({ kind: "rows", items: [item] });
    }
    return blocks.map((b, bi) => {
      if (b.kind === "section") {
        const it = b.item;
        if (!isSectionId(it.id)) return null;
        const d = data.get(it.workspace_id);
        if (!d) return null;
        if (d.open)
          return (
            <SectionMarkCtx.Provider key={itemKey(it)} value={markFor(it.workspace_id)}>
              {renderSection(it.id, bi === 0)}
            </SectionMarkCtx.Provider>
          );
        return <RemoteSection key={itemKey(it)} d={d} id={it.id} first={bi === 0} rail={rail} mark={markFor(it.workspace_id)} ctx={{ labels }} />;
      }
      return (
        <div key={`rows-${bi}`} className={cn("flex flex-col gap-px", bi > 0 && "mt-3")}>
          {b.items.map((item) => {
            const d = data.get(item.workspace_id);
            if (!d) return null;
            const model = buildRow(item, d, { labels, here: d.open ? here : undefined, waiting: d.open ? waitingFor : undefined });
            if (!model) return null;
            const key = itemKey(item);
            const rk = `${section.id}|${key}`;
            if (renaming === rk && !rail)
              return (
                <RenameRow
                  key={key}
                  initial={item.name ?? model.label}
                  placeholder={model.label}
                  label={t("row.renameLabel", { name: model.label })}
                  onDone={(v) => {
                    setRenaming(null);
                    if (v === null) return;
                    const name = v.trim();
                    change(updateItem(space, key, { name: !name || name === model.label ? null : name }));
                  }}
                />
              );
            return (
              <NavItemRow
                key={key}
                model={model}
                rail={rail}
                prefs={d.prefs}
                own={item.badge}
                label={item.name}
                ws={d.open ? undefined : d.id}
                mark={markFor(item.workspace_id)}
                menu={(row) => (
                  <SpaceRowMenu spaces={spaces} space={space} item={item} model={model} put={actions.put} onRename={() => setRenaming(rk)} rail={rail} label={item.name || model.label}>
                    {row}
                  </SpaceRowMenu>
                )}
              />
            );
          })}
        </div>
      );
    });
  };

  return (
    <>
      {shown.sections.map(({ section, items }, si) => (
        <SpaceSectionBlock key={section.id} section={section} first={si === 0} rail={rail} onToggle={() => change(updateSection(space, section.id, { collapsed: !section.collapsed }))}>
          {renderItems(section, items)}
        </SpaceSectionBlock>
      ))}
      {!shown.sections.length && !rail && (
        <div className="mx-2.5 rounded-md bg-chrome-hover px-2.5 py-2 text-[11.5px] leading-4 text-fg-3">
          <p>{t("view.empty")}</p>
          <button
            type="button"
            onClick={() => spaceUi.edit({ kind: "space", id: space.id })}
            className="mt-1 inline-flex items-center gap-1 rounded font-medium text-fg-2 outline-none hover:text-fg focus-visible:shadow-[0_0_0_2px_var(--focus)]"
          >
            <Plus className="size-3" />
            {t("view.emptyAdd")}
          </button>
        </div>
      )}
    </>
  );
}
