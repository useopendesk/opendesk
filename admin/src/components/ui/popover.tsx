import { Popover as P } from "radix-ui";
import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export const floatingPanel =
  "z-50 origin-[var(--radix-popper-transform-origin)] rounded-lg border border-line bg-raised text-fg shadow-pop outline-none data-[state=open]:animate-pop-in data-[state=closed]:animate-pop-out";

export function Popover({
  trigger,
  children,
  open,
  onOpenChange,
  align = "start",
  side = "bottom",
  sideOffset = 6,
  className,
  modal,
}: {
  trigger: ReactNode;
  children: ReactNode;
  open?: boolean;
  onOpenChange?: (o: boolean) => void;
  align?: "start" | "center" | "end";
  side?: "top" | "bottom" | "left" | "right";
  sideOffset?: number;
  className?: string;
  modal?: boolean;
}) {
  return (
    <P.Root open={open} onOpenChange={onOpenChange} modal={modal}>
      <P.Trigger asChild>{trigger}</P.Trigger>
      <P.Portal>
        <P.Content
          align={align}
          side={side}
          sideOffset={sideOffset}
          collisionPadding={8}
          className={cn(floatingPanel, "origin-[var(--radix-popover-content-transform-origin)]", className)}
          onClick={(e) => e.stopPropagation()}
        >
          {children}
        </P.Content>
      </P.Portal>
    </P.Root>
  );
}
