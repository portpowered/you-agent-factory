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
        "relative flex w-px items-center justify-center bg-af-border outline-none transition-colors focus-visible:bg-af-border-strong data-[panel-group-direction=vertical]:h-px data-[panel-group-direction=vertical]:w-full",
        className,
      )}
      {...props}
    >
      {withHandle ? (
        <div className="flex h-10 w-5 items-center justify-center rounded-full border border-af-border bg-af-surface-raised text-af-text-subtle">
          <GripVerticalIcon className="h-4 w-4" />
        </div>
      ) : null}
    </Separator>
  );
}
