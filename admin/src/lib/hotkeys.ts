import { useEffect, useRef } from "react";

export type HotkeyMap = Record<string, (e: KeyboardEvent) => void>;

export function isTypingTarget(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  if (!el || !el.tagName) return false;
  if (el.isContentEditable) return true;
  const tag = el.tagName;
  if (tag === "TEXTAREA" || tag === "SELECT") return true;
  if (tag === "INPUT") {
    const type = (el as HTMLInputElement).type;
    return !["checkbox", "radio", "button", "submit"].includes(type);
  }
  return false;
}

function modalOpen() {
  return !!document.querySelector('[role="dialog"][aria-modal="true"], [role="alertdialog"], [data-radix-popper-content-wrapper]');
}

/** Normalised name for a key event: "mod+k", "shift+?", "j", "enter", "escape". */
export function keyName(e: KeyboardEvent): string {
  const parts: string[] = [];
  if (e.metaKey || e.ctrlKey) parts.push("mod");
  if (e.altKey) parts.push("alt");
  let k = e.key.toLowerCase();
  if (k === " ") k = "space";
  if (e.shiftKey && k.length > 1) parts.push("shift");
  parts.push(k);
  return parts.join("+");
}

/**
 * Page-level single-key shortcuts. Ignored while typing or while a dialog or
 * popover is open, except bindings that use the mod key. "g w"-style
 * sequences are supported.
 */
export function useHotkeys(map: HotkeyMap, enabled = true) {
  const ref = useRef(map);
  useEffect(() => {
    ref.current = map;
  });
  useEffect(() => {
    if (!enabled) return;
    let pending: string | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing) return;
      const name = keyName(e);
      const withMod = name.startsWith("mod+");
      if (!withMod && (isTypingTarget(e.target) || modalOpen())) return;
      const seq = pending ? `${pending} ${name}` : null;
      const fn = (seq && ref.current[seq]) || ref.current[name];
      if (fn) {
        pending = null;
        e.preventDefault();
        fn(e);
        return;
      }
      if (!withMod && Object.keys(ref.current).some((k) => k.startsWith(`${name} `))) {
        pending = name;
        clearTimeout(timer);
        timer = setTimeout(() => (pending = null), 900);
      } else pending = null;
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      clearTimeout(timer);
    };
  }, [enabled]);
}
