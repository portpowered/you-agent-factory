import { DayPicker, getDefaultClassNames } from "react-day-picker";
import type { ComponentProps } from "react";

import { cn } from "../../lib/cn";
import { buttonVariants } from "./button";

export type CalendarProps = ComponentProps<typeof DayPicker>;

export function Calendar({ className, classNames, showOutsideDays = true, ...props }: CalendarProps) {
  const defaultClassNames = getDefaultClassNames();

  return (
    <DayPicker
      className={cn("rounded-2xl border border-af-border bg-af-surface-subtle p-3", className)}
      classNames={{
        button_next: cn(buttonVariants({ size: "icon", tone: "ghost" }), "h-9 w-9"),
        button_previous: cn(buttonVariants({ size: "icon", tone: "ghost" }), "h-9 w-9"),
        caption_label: "font-semibold text-af-text",
        chevron: "fill-none stroke-current text-af-text-muted",
        day: "h-10 w-10 p-0 font-medium text-af-text aria-selected:opacity-100",
        day_button: cn(
          "h-10 w-10 rounded-lg text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-af-focus-ring",
          "hover:bg-af-overlay aria-selected:bg-af-accent aria-selected:text-af-on-accent",
        ),
        disabled: "text-af-text-disabled",
        month: "space-y-4",
        nav: "flex items-center gap-2",
        outside: "text-af-text-disabled aria-selected:bg-af-accent-surface aria-selected:text-af-on-accent",
        root: cn(defaultClassNames.root, "text-sm"),
        selected: "font-semibold",
        today: "text-af-accent",
        weekday: "text-xs font-bold uppercase tracking-[0.08em] text-af-text-subtle",
        ...classNames,
      }}
      showOutsideDays={showOutsideDays}
      {...props}
    />
  );
}
