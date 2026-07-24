import type { CSSProperties } from "react";

import { cn } from "../utilities/cn";

import { WIDGET_FRAME_SUPPORTING_LABELS_CLASS } from "./widget-frame-typography";

export const WIDGET_FRAME_MIN_WIDTH_CLASS = "min-w-0";

export const WIDGET_FRAME_WIDE_BODY_CLASS = "min-h-72";

export const WIDGET_FRAME_RESPONSIVE_SHELL_CLASS = "min-w-0 w-full";

export const WIDGET_FRAME_STORY_SHELL_DATA_ATTR =
  "data-widget-frame-story-shell";

export const WIDGET_FRAME_OVERFLOW_TOLERANCE_PX = 1;

/** Detail-card layout recipe for description lists inside widget frames. */
export const widgetFrameDetailCardClass = cn(
  "[&_dd]:m-0 [&_dl]:m-0 [&_dl]:grid [&_dl]:gap-3 [&_dl_div:first-child]:border-t-0 [&_dl_div:first-child]:pt-0 [&_dl_div]:border-t [&_dl_div]:border-outline [&_dl_div]:pt-3 [&_dt]:mb-1 [&_h3]:mt-0",
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
);

export function widgetFrameStoryShellStyle(maxWidth: string): {
  style: CSSProperties;
} {
  return {
    style: {
      maxWidth,
      padding: "1rem",
      width: "100%",
    },
  };
}

export function widgetFrameHasNoHorizontalOverflow(
  element: HTMLElement,
  tolerancePx = WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
): boolean {
  return element.scrollWidth <= element.clientWidth + tolerancePx;
}
