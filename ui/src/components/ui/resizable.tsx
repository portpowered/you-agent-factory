import { GripVerticalIcon } from "./resizable-icons";
import type { ComponentProps } from "react";
import { cn } from "../../lib/cn";
import { Group, Panel, Separator } from "react-resizable-panels";

export { Panel as ResizablePanel, Group as ResizablePanelGroup };

export function ResizableHandle({
  className,
  withHandle = false,
  ...props
}: ComponentProps<typeof Separator> & { withHandle?: boolean }) {
  return (
    <Separator
      className={cn(
        "relative flex w-px items-center justify-center bg-af-overlay/12 outline-none transition-colors focus-visible:bg-af-accent/45 data-[panel-group-direction=vertical]:h-px data-[panel-group-direction=vertical]:w-full",
        className,
      )}
      {...props}
    >
      {withHandle ? (
        <div className="flex h-10 w-5 items-center justify-center rounded-full border border-af-overlay/12 bg-af-surface/94 text-af-ink/58">
          <GripVerticalIcon className="h-4 w-4" />
        </div>
      ) : null}
    </Separator>
  );
}
