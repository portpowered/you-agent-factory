import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../utilities/cn";

type CodePanelPadding = "compact" | "default";
type CodePanelSurface = "high" | "low";

export interface CodePanelProps extends HTMLAttributes<HTMLPreElement> {
  padding?: CodePanelPadding;
  surface?: CodePanelSurface;
}

const CODE_PANEL_PADDING_CLASS: Record<CodePanelPadding, string> = {
  compact: "p-2",
  default: "p-3",
};
const CODE_PANEL_SURFACE_CLASS: Record<CodePanelSurface, string> = {
  high: "bg-surface-container-high",
  low: "bg-surface-container-low",
};

const CODE_PANEL_BODY_CODE_CLASS = "font-mono text-code-medium text-code";

export function codePanelVariants({
  className,
  padding = "compact",
  surface = "high",
}: Pick<CodePanelProps, "className" | "padding" | "surface">) {
  return cn(
    "m-0 max-w-full overflow-x-auto whitespace-pre-wrap rounded-lg border border-outline [overflow-wrap:anywhere]",
    CODE_PANEL_PADDING_CLASS[padding],
    CODE_PANEL_SURFACE_CLASS[surface],
    CODE_PANEL_BODY_CODE_CLASS,
    className,
  );
}

export const CodePanel = forwardRef<HTMLPreElement, CodePanelProps>(
  function CodePanel({ className, padding, surface, ...props }, ref) {
    return (
      <pre
        className={codePanelVariants({ className, padding, surface })}
        ref={ref}
        {...props}
      />
    );
  },
);
