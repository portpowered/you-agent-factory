import * as ScrollAreaPrimitive from "@radix-ui/react-scroll-area";
import { type ComponentProps, type ComponentRef, forwardRef } from "react";

import { cn } from "../../lib/cn";

const SCROLL_AREA_VIEWPORT_CLASS = cn(
  "size-full outline-none",
  "[&>div]:!block [&>div]:size-full",
  "[scrollbar-width:none] [-ms-overflow-style:none]",
  "[&::-webkit-scrollbar]:hidden",
);

export const SCROLL_AREA_THUMB_CLASS =
  "relative flex-1 rounded-full bg-outline-variant transition-colors hover:bg-af-text-subtle";

export type ScrollAreaProps = ComponentProps<
  typeof ScrollAreaPrimitive.Root
> & {
  viewportClassName?: string;
  viewportProps?: ComponentProps<typeof ScrollAreaPrimitive.Viewport>;
};

export const ScrollArea = forwardRef<
  ComponentRef<typeof ScrollAreaPrimitive.Root>,
  ScrollAreaProps
>(function ScrollArea(
  {
    children,
    className,
    type = "auto",
    viewportClassName,
    viewportProps,
    ...props
  },
  ref,
) {
  return (
    <ScrollAreaPrimitive.Root
      className={cn("relative overflow-hidden", className)}
      ref={ref}
      type={type}
      {...props}
    >
      <ScrollAreaPrimitive.Viewport
        className={cn(SCROLL_AREA_VIEWPORT_CLASS, viewportClassName)}
        {...viewportProps}
      >
        {children}
      </ScrollAreaPrimitive.Viewport>
      <ScrollBar />
      <ScrollAreaPrimitive.Corner />
    </ScrollAreaPrimitive.Root>
  );
});

export type ScrollBarProps = ComponentProps<
  typeof ScrollAreaPrimitive.ScrollAreaScrollbar
>;

export const ScrollBar = forwardRef<
  ComponentRef<typeof ScrollAreaPrimitive.ScrollAreaScrollbar>,
  ScrollBarProps
>(function ScrollBar({ className, orientation = "vertical", ...props }, ref) {
  return (
    <ScrollAreaPrimitive.ScrollAreaScrollbar
      className={cn(
        "flex touch-none select-none bg-transparent p-px transition-colors",
        orientation === "vertical" &&
          "h-full w-2.5 border-l border-l-transparent",
        orientation === "horizontal" &&
          "h-2.5 flex-col border-t border-t-transparent",
        className,
      )}
      orientation={orientation}
      ref={ref}
      {...props}
    >
      <ScrollAreaPrimitive.ScrollAreaThumb
        className={SCROLL_AREA_THUMB_CLASS}
      />
    </ScrollAreaPrimitive.ScrollAreaScrollbar>
  );
});
