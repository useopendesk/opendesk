// Keeps an editor suggestion list ("{{", "/", "@") attached to the caret: it
// follows scrolling and resizing, hides while the caret is scrolled out of
// view, and closes on a click outside it or when the window loses focus.

import { useLayoutEffect, useState, type RefObject } from "react";

/** The part of the viewport where el is visible: its scrolling ancestors clip it. */
function visibleBox(el: HTMLElement | null): { top: number; bottom: number; left: number; right: number } {
  const box = { top: 0, bottom: window.innerHeight, left: 0, right: window.innerWidth };
  for (let n = el?.parentElement ?? null; n && n !== document.body; n = n.parentElement) {
    const { overflowY, overflowX } = getComputedStyle(n);
    if (/(auto|scroll|hidden|clip)/.test(overflowY + overflowX)) {
      const r = n.getBoundingClientRect();
      box.top = Math.max(box.top, r.top);
      box.bottom = Math.min(box.bottom, r.bottom);
      box.left = Math.max(box.left, r.left);
      box.right = Math.min(box.right, r.right);
    }
  }
  return box;
}

/**
 * The caret's rect while the list should show, else null. `onDismiss` runs on a
 * click outside the list or when the window loses focus.
 */
export function useCaretAnchor({
  open,
  getRect,
  anchorEl,
  popupRef,
  onDismiss,
}: {
  open: boolean;
  getRect: (() => DOMRect | null) | null;
  anchorEl: HTMLElement | null;
  popupRef: RefObject<HTMLElement | null>;
  onDismiss: () => void;
}): DOMRect | null {
  const [rect, setRect] = useState<DOMRect | null>(null);

  useLayoutEffect(() => {
    if (!open || !getRect) {
      setRect(null);
      return;
    }
    let frame = 0;
    const measure = () => {
      const r = getRect();
      if (!r) return setRect(null);
      const box = visibleBox(anchorEl);
      // The caret scrolled out of its editor or the window: nothing to point at.
      const inView = r.bottom > box.top && r.top < box.bottom && r.right >= box.left && r.left <= box.right;
      setRect(inView ? r : null);
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    };
    measure();
    // After the click: a drawer around the editor still sees the list open and stays,
    // so one click outside closes the list, the next one the drawer.
    const down = (e: PointerEvent) => {
      if (!popupRef.current?.contains(e.target as Node)) setTimeout(onDismiss, 0);
    };
    window.addEventListener("scroll", schedule, true);
    window.addEventListener("resize", schedule);
    window.addEventListener("blur", onDismiss);
    document.addEventListener("pointerdown", down, true);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule, true);
      window.removeEventListener("resize", schedule);
      window.removeEventListener("blur", onDismiss);
      document.removeEventListener("pointerdown", down, true);
    };
  }, [open, getRect, anchorEl, popupRef, onDismiss]);

  return rect;
}

/** Where a list of the given size goes next to the caret: below when it fits, else above. */
export function placeBelowOrAbove(rect: DOMRect, width: number, height: number): React.CSSProperties {
  const below = rect.bottom + height + 12 < window.innerHeight || rect.top < height + 12;
  return {
    position: "fixed",
    left: Math.max(8, Math.min(rect.left, window.innerWidth - width - 8)),
    top: below ? rect.bottom + 6 : undefined,
    bottom: below ? undefined : window.innerHeight - rect.top + 6,
  };
}
