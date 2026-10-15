import { useEffect, useState } from "react";
import { TextInput } from "@/components/ui/input";
import { cn } from "@/lib/cn";

/** A port number: digits only, 1 to 65535, no spinner. */
export function PortInput({
  id,
  value,
  onChange,
  invalid,
  className,
  "aria-describedby": describedBy,
}: {
  id?: string;
  value: number;
  onChange: (n: number) => void;
  invalid?: boolean;
  className?: string;
  "aria-describedby"?: string;
}) {
  const [text, setText] = useState(value ? String(value) : "");
  useEffect(() => {
    setText((t) => (Number(t) === value ? t : value ? String(value) : ""));
  }, [value]);
  const n = Number(text);
  const bad = text !== "" && !(Number.isInteger(n) && n >= 1 && n <= 65535);
  return (
    <TextInput
      id={id}
      inputMode="numeric"
      autoComplete="off"
      maxLength={5}
      className={cn("mono tabular-nums", className)}
      invalid={invalid || bad || text === ""}
      aria-describedby={describedBy}
      value={text}
      onChange={(e) => {
        const next = e.target.value.replace(/\D/g, "").slice(0, 5);
        setText(next);
        const v = Number(next);
        onChange(next && v >= 1 && v <= 65535 ? v : 0);
      }}
    />
  );
}

/** A port the API accepts. */
export const validPort = (n: number) => Number.isInteger(n) && n >= 1 && n <= 65535;
