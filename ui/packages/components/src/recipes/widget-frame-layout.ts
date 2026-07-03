import { cn } from "../utilities/cn";

import { WIDGET_FRAME_SUPPORTING_LABELS_CLASS } from "./widget-frame-typography";

export const WIDGET_FRAME_MIN_WIDTH_CLASS = "min-w-0";

export const WIDGET_FRAME_WIDE_BODY_CLASS = "min-h-72";

/** Detail-card layout recipe for description lists inside widget frames. */
export const widgetFrameDetailCardClass = cn(
  "[&_dd]:m-0 [&_dl]:m-0 [&_dl]:grid [&_dl]:gap-3 [&_dl_div:first-child]:border-t-0 [&_dl_div:first-child]:pt-0 [&_dl_div]:border-t [&_dl_div]:border-outline [&_dl_div]:pt-3 [&_dt]:mb-1 [&_h3]:mt-0",
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
);
