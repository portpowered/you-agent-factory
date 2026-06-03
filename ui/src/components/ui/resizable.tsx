import type { ComponentProps } from "react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { cn } from "../../lib/cn";
import { GripVerticalIcon } from "./resizable-icons";

export { Group as ResizablePanelGroup, Panel as ResizablePanel };

export function ResizableHandle({
  className,
  withHandle = false,
  ...props
}: ComponentProps<typeof Separator> & { withHandle?: boolean }) {
  return (
    <Separator
      className={cn(
        "relative flex w-px items-center justify-center bg-outline outline-none transition-colors focus-visible:bg-outline-variant data-[panel-group-direction=vertical]:h-px data-[panel-group-direction=vertical]:w-full",
        className,
      )}
      {...props}
    >
      {withHandle ? (
        <div className="flex h-10 w-5 items-center justify-center rounded-full border border-outline bg-surface-container-high text-af-text-subtle">
          <GripVerticalIcon className="h-4 w-4" />
        </div>
      ) : null}
    </Separator>
  );
}
