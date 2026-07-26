import type { ButtonHTMLAttributes } from "react";

import { Button } from "../../../../../components/ui/button";
import { cn } from "../../../../../lib/cn";

export interface CurrentSelectionTraceButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onClick"> {
  activeTraceID?: string | null;
  onSelectTraceID?: (traceID: string) => void;
  selectedTraceSuffix?: string;
  traceID: string;
}

export function CurrentSelectionTraceButton({
  activeTraceID,
  children,
  className,
  onSelectTraceID,
  selectedTraceSuffix = "",
  traceID,
  ...props
}: CurrentSelectionTraceButtonProps) {
  const selected = activeTraceID === traceID;

  return (
    <Button
      aria-pressed={selected}
      className={cn("w-fit", className)}
      onClick={() => onSelectTraceID?.(traceID)}
      size="sm"
      tone="outline"
      {...props}
    >
      {children ?? traceID}
      {selected ? selectedTraceSuffix : ""}
    </Button>
  );
}
