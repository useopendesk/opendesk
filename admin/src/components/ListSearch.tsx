import { useEffect, useRef, useState } from "react";
import { SearchInput } from "@/components/ui/input";
import { Kbd } from "@/components/ui/kbd";

/**
 * The page's search box: typing updates the field at once and the URL (and so the
 * query) after a short pause. "/" focuses it from anywhere on the page.
 */
export function ListSearch({ value, onCommit, placeholder, delay = 250 }: { value: string | undefined; onCommit: (v: string | undefined) => void; placeholder: string; delay?: number }) {
  const [text, setText] = useState(value ?? "");
  const commit = useRef(onCommit);
  useEffect(() => {
    commit.current = onCommit;
  });
  // The URL changed from outside (back, a cleared filter): follow it.
  const [seen, setSeen] = useState(value);
  if (seen !== value) {
    setSeen(value);
    if ((value ?? "") !== text.trim()) setText(value ?? "");
  }
  useEffect(() => {
    const v = text.trim() || undefined;
    if (v === (value || undefined)) return;
    const t = setTimeout(() => commit.current(v), delay);
    return () => clearTimeout(t);
  }, [text, value, delay]);
  return (
    <div className="relative w-full max-w-[340px]">
      <SearchInput pageSearch value={text} onChange={setText} placeholder={placeholder} className="w-full" />
      {!text && (
        <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2">
          <Kbd>/</Kbd>
        </span>
      )}
    </div>
  );
}
