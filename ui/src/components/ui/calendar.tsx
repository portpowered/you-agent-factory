import { DayPicker, getDefaultClassNames } from "react-day-picker";
import type { ComponentProps } from "react";

import { cn } from "../../lib/cn";
import { buttonVariants } from "./button";

export type CalendarProps = ComponentProps<typeof DayPicker>;

export function Calendar({ className, classNames, showOutsideDays = true, ...props }: CalendarProps) {
  const defaultClassNames = getDefaultClassNames();

  return (
    <DayPicker
      className={cn("rounded-2xl border border-af-overlay/10 bg-af-surface/60 p-3", className)}
      classNames={{
        button_next: cn(buttonVariants({ size: "icon", tone: "ghost" }), "h-9 w-9"),
        button_previous: cn(buttonVariants({ size: "icon", tone: "ghost" }), "h-9 w-9"),
        caption_label: "font-semibold text-af-ink",
        chevron: "fill-none stroke-current text-af-ink/72",
        day: "h-10 w-10 p-0 font-medium text-af-ink aria-selected:opacity-100",
        day_button: cn(
          "h-10 w-10 rounded-lg text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-accent/25",
          "hover:bg-af-overlay/8 aria-selected:bg-af-accent aria-selected:text-af-canvas",
        ),
        disabled: "text-af-ink/30",
        month: "space-y-4",
        nav: "flex items-center gap-2",
        outside: "text-af-ink/36 aria-selected:bg-af-accent/40 aria-selected:text-af-canvas",
        root: cn(defaultClassNames.root, "text-sm"),
        selected: "font-semibold",
        today: "text-af-accent",
        weekday: "text-xs font-bold uppercase tracking-[0.08em] text-af-ink/58",
        ...classNames,
      }}
      showOutsideDays={showOutsideDays}
      {...props}
    />
  );
}
