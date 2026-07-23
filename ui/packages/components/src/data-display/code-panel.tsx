import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "../utilities/cn";

type CodePanelPadding = "compact" | "default";
type CodePanelSurface = "high" | "low";
type CodePanelMaxHeight = "none" | "sm" | "md" | "lg";

export interface CodePanelProps extends HTMLAttributes<HTMLPreElement> {
  maxHeight?: CodePanelMaxHeight;
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
const CODE_PANEL_MAX_HEIGHT_CLASS: Record<CodePanelMaxHeight, string> = {
  none: "",
  sm: "max-h-48 overflow-y-auto",
  md: "max-h-72 overflow-y-auto",
  lg: "max-h-96 overflow-y-auto",
};

const CODE_PANEL_BODY_CODE_CLASS = "font-mono text-code-medium text-code";

const CODE_PANEL_CONTAINMENT_CLASS =
  "m-0 min-w-0 w-full max-w-full overflow-x-auto whitespace-pre-wrap rounded-lg border border-outline [overflow-wrap:anywhere] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary";

export function codePanelVariants({
  className,
  maxHeight = "none",
  padding = "compact",
  surface = "high",
}: Pick<CodePanelProps, "className" | "maxHeight" | "padding" | "surface">) {
  return cn(
    CODE_PANEL_CONTAINMENT_CLASS,
    CODE_PANEL_PADDING_CLASS[padding],
    CODE_PANEL_SURFACE_CLASS[surface],
    CODE_PANEL_MAX_HEIGHT_CLASS[maxHeight],
    CODE_PANEL_BODY_CODE_CLASS,
    className,
  );
}

export const CodePanel = forwardRef<HTMLPreElement, CodePanelProps>(
  function CodePanel(
    { className, maxHeight = "none", padding, surface, tabIndex, ...props },
    ref,
  ) {
    const isScrollable = maxHeight !== "none";

    return (
      <pre
        className={codePanelVariants({
          className,
          maxHeight,
          padding,
          surface,
        })}
        ref={ref}
        tabIndex={tabIndex ?? (isScrollable ? 0 : undefined)}
        {...props}
      />
    );
  },
);
