// The team's discussion of a conversation: comments only teammates who can see it
// read, never the sender. @ mentions a teammate (the ticket notes' mention list);
// a mention of someone who cannot see it shares the conversation with them.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Lock, MoreHorizontal, Pencil, Trash2 } from "lucide-react";
import { forwardRef, useImperativeHandle, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api/endpoints";
import type { MailComment, MailThreadDetail, Member } from "@/lib/api/types";
import { isBlankHtml, mentionIds } from "@/lib/collab/html";
import { fullDate, relShort } from "@/lib/format";
import { cn } from "@/lib/cn";
import { qk, q } from "@/lib/queries";
import { sanitizeHtml } from "@/lib/sanitize";
import { atLeast, useRole, useSession } from "@/lib/session";
import { RichEditor, type RichEditorHandle } from "@/components/editor/RichEditor";
import { mentionExtension, SuggestionPopup, useSuggest } from "@/components/tickets/detail/suggestions";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { useConfirm } from "@/components/ui/confirm";
import { Menu, MenuContent, MenuItem, MenuTrigger } from "@/components/ui/menu";
import { toast } from "@/components/ui/toast";
import { Tooltip } from "@/components/ui/tooltip";

function useMentionExtension() {
  const { data: members } = useQuery(q.members());
  const ref = useRef<Member[]>([]);
  ref.current = members ?? [];
  // One extension per editor; it reads the members as they are when @ is typed.
  return useMemo(() => [mentionExtension(() => ref.current)], []);
}

function patchComments(qc: ReturnType<typeof useQueryClient>, threadId: string, fn: (c: MailComment[]) => MailComment[]) {
  qc.setQueryData<MailThreadDetail>(qk.mailThread(threadId), (old) => {
    if (!old) return old;
    const comments = fn(old.comments);
    return { ...old, comments, thread: { ...old.thread, comment_count: comments.length } };
  });
}

function CommentItem({ c, threadId, canAct }: { c: MailComment; threadId: string; canAct: boolean }) {
  const { t } = useTranslation("collab");
  const qc = useQueryClient();
  const confirm = useConfirm();
  const me = useSession((s) => s.user?.id);
  const admin = atLeast(useRole(), "admin");
  const mine = !!c.author && c.author.id === me;
  const [editing, setEditing] = useState(false);
  const [html, setHtml] = useState(c.body_html);
  const extensions = useMentionExtension();
  const editor = useRef<RichEditorHandle>(null);
  const body = useMemo(() => sanitizeHtml(c.body_html), [c.body_html]);
  const name = c.author?.name ?? t("comments.someone");

  const save = useMutation({
    mutationFn: (body_html: string) => api.collab.comments.update(c.id, body_html),
    onSuccess: (n) => {
      patchComments(qc, threadId, (list) => list.map((x) => (x.id === n.id ? n : x)));
      setEditing(false);
    },
    onError: (e) => toast.error(e),
  });
  const remove = () =>
    confirm.show(
      t("comments.deleteConfirm"),
      async () => {
        await api.collab.comments.remove(c.id);
        patchComments(qc, threadId, (list) => list.filter((x) => x.id !== c.id));
      },
      { title: t("comments.deleteTitle"), confirmLabel: t("comments.delete"), danger: true },
    );

  return (
    <li id={`comment-${c.id}`} className="group/comment flex scroll-mt-4 gap-2.5">
      <Avatar name={name} src={c.author?.avatar_url} size="sm" className="mt-0.5" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="truncate text-[12.5px] font-medium">{name}</span>
          <Tooltip content={fullDate(c.created_at)}>
            <time dateTime={c.created_at} className="mono shrink-0 text-[10.5px] text-fg-4">
              {relShort(c.created_at)}
            </time>
          </Tooltip>
          {c.updated_at !== c.created_at && <span className="text-[10.5px] text-fg-4">{t("comments.edited")}</span>}
          {canAct && (mine || admin) && !editing && (
            <Menu>
              <MenuTrigger asChild>
                <button
                  type="button"
                  aria-label={t("comments.menu")}
                  className="ml-auto flex size-6 items-center justify-center rounded-md text-fg-4 opacity-0 transition-opacity hover:bg-hover hover:text-fg focus-visible:opacity-100 group-hover/comment:opacity-100 data-[state=open]:opacity-100"
                >
                  <MoreHorizontal className="size-3.5" />
                </button>
              </MenuTrigger>
              <MenuContent>
                {mine && (
                  <MenuItem icon={<Pencil />} onSelect={() => setEditing(true)}>
                    {t("comments.edit")}
                  </MenuItem>
                )}
                <MenuItem icon={<Trash2 />} danger onSelect={remove}>
                  {t("comments.delete")}
                </MenuItem>
              </MenuContent>
            </Menu>
          )}
        </div>
        {editing ? (
          <div className="mt-1 rounded-md border border-line bg-surface">
            <RichEditor
              ref={editor}
              value={html}
              onChange={setHtml}
              extensions={extensions}
              autofocus
              toolbar="none"
              contentClassName="min-h-[40px] text-[12.5px]"
              ariaLabel={t("comments.edit")}
              onSubmit={() => !isBlankHtml(html) && save.mutate(html)}
            />
            <div className="flex justify-end gap-1.5 px-2 pb-2">
              <Button size="xs" variant="ghost" onClick={() => setEditing(false)}>
                {t("comments.cancel")}
              </Button>
              <Button size="xs" variant="primary" loading={save.isPending} disabled={isBlankHtml(html)} onClick={() => save.mutate(html)}>
                {t("comments.save")}
              </Button>
            </div>
          </div>
        ) : (
          <div className="prose-od mt-0.5 text-[12.5px] text-fg" data-ph-mask dangerouslySetInnerHTML={{ __html: body }} />
        )}
      </div>
    </li>
  );
}

/** The comments, in the conversation under its messages; written in the pinned `CommentBar`. */
export function Comments({ detail }: { detail: MailThreadDetail }) {
  const { t } = useTranslation("collab");
  if (!detail.comments.length) return null;
  return (
    <section aria-label={t("comments.title")} className="rounded-lg border border-dashed border-note-line bg-note px-4 py-3">
      <div className="flex items-center gap-1.5">
        <Lock className="size-3 text-fg-3" />
        <h3 className="text-[12px] font-semibold tracking-[-0.01em]">{t("comments.title")}</h3>
        <span className="mono text-[10.5px] text-fg-4">{detail.comments.length}</span>
      </div>
      <p className="mt-0.5 text-[11.5px] text-fg-3">{t("comments.hint")}</p>
      <ul className="mt-3 flex flex-col gap-3">
        {detail.comments.map((c) => (
          <CommentItem key={c.id} c={c} threadId={detail.thread.id} canAct={detail.can_act} />
        ))}
      </ul>
    </section>
  );
}

export type CommentBarHandle = { focus: () => void };

/**
 * Writing a comment, pinned under the conversation so it is there however long the
 * mail is: one line until it is used, then the editor, who reads it and the buttons.
 * `children` sit beside the line (the reply buttons) and step aside while writing.
 */
export const CommentBar = forwardRef<CommentBarHandle, { detail: MailThreadDetail; onPosted?: (id: string) => void; children?: ReactNode }>(function CommentBar({ detail, onPosted, children }, ref) {
  const { t } = useTranslation("collab");
  const qc = useQueryClient();
  const threadId = detail.thread.id;
  const [html, setHtml] = useState("");
  const [focused, setFocused] = useState(false);
  const editor = useRef<RichEditorHandle>(null);
  const extensions = useMentionExtension();
  const writing = focused || !isBlankHtml(html);
  const add = useMutation({
    mutationFn: (body_html: string) => api.collab.threads.comment(threadId, { body_html, mentions: mentionIds(body_html) }),
    onSuccess: (c) => {
      patchComments(qc, threadId, (list) => (list.some((x) => x.id === c.id) ? list : [...list, c]));
      editor.current?.clear();
      setHtml("");
      onPosted?.(c.id);
    },
    onError: (e) => toast.error(e),
  });
  const submit = () => {
    const body = editor.current?.editor?.getHTML() ?? html;
    if (!isBlankHtml(body) && !add.isPending) add.mutate(body);
  };
  const leave = () => {
    editor.current?.editor?.commands.blur();
    setFocused(false);
  };
  useImperativeHandle(ref, () => ({ focus: () => editor.current?.focus() }), []);

  return (
    <div className="flex items-center gap-1.5">
      <div
        className={cn(
          "min-w-0 flex-1 rounded-md border border-dashed border-note-line bg-note transition-colors duration-150 hover:border-line-strong",
          writing && "border-solid border-line-strong bg-surface",
        )}
        onBlur={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setFocused(false);
        }}
      >
        <div className="flex items-start">
          <Lock aria-hidden className="ml-2.5 mt-[9px] size-3 shrink-0 text-fg-4" />
          <RichEditor
            ref={editor}
            value={html}
            onChange={setHtml}
            extensions={extensions}
            toolbar="none"
            placeholder={t("comments.placeholder")}
            ariaLabel={t("comments.placeholder")}
            className="max-h-[40vh] min-w-0 flex-1"
            contentClassName={cn("min-h-0 px-2 py-[5px] text-[12.5px] leading-5", writing && "min-h-[52px]")}
            onFocus={() => setFocused(true)}
            onKeyDown={(e) => {
              // Escape leaves the comment as it is (a mention list open closes first).
              if (e.key !== "Escape" || useSuggest.getState().open) return false;
              leave();
              return true;
            }}
            onSubmit={submit}
          />
        </div>
        {writing && (
          <div className="flex items-center gap-2 pb-2 pl-[30px] pr-2">
            <p className="min-w-0 flex-1 truncate text-[11px] text-fg-4">{t("comments.hint")}</p>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => {
                editor.current?.clear();
                setHtml("");
                leave();
              }}
            >
              {t("comments.cancel")}
            </Button>
            <Tooltip content={t("comments.add")} shortcut="mod enter">
              <Button size="xs" variant="primary" loading={add.isPending} disabled={isBlankHtml(html)} onClick={submit}>
                {t("comments.add")}
              </Button>
            </Tooltip>
          </div>
        )}
      </div>
      {!writing && children}
      <SuggestionPopup />
    </div>
  );
});
