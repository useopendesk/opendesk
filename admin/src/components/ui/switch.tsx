import { Switch as S } from "radix-ui";
import { cn } from "@/lib/cn";

export function Switch({
  checked,
  onCheckedChange,
  disabled,
  id,
  className,
  "aria-label": ariaLabel,
}: {
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
  disabled?: boolean;
  id?: string;
  className?: string;
  "aria-label"?: string;
}) {
  return (
    <S.Root
      id={id}
      checked={checked}
      disabled={disabled}
      aria-label={ariaLabel}
      onClick={(e) => e.stopPropagation()}
      onCheckedChange={onCheckedChange}
      className={cn(
        "group relative inline-flex h-[18px] w-[30px] shrink-0 items-center rounded-full border border-transparent bg-line-strong transition-[background-color,box-shadow] duration-150 outline-none focus-visible:shadow-[0_0_0_1px_var(--surface),0_0_0_3px_var(--focus)] data-[state=checked]:bg-inverse disabled:opacity-50",
        className,
      )}
    >
      <S.Thumb className="block size-[14px] translate-x-[1px] rounded-full bg-surface shadow-[0_1px_2px_rgb(0_0_0/0.25)] transition-[transform,width] duration-200 ease-[cubic-bezier(0.34,1.56,0.64,1)] group-active:w-[16px] data-[state=checked]:translate-x-[13px] group-active:data-[state=checked]:translate-x-[11px] motion-reduce:transition-none dark:data-[state=checked]:bg-inverse-fg" />
    </S.Root>
  );
}
