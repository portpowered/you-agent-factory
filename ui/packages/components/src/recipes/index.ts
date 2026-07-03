/** Stable category path for `@you-agent-factory/components/recipes`. */
export const COMPONENTS_CATEGORY = "recipes" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetSubtitle,
} from "./widget-frame-content";
export type {
  WidgetDetailCopyProps,
  WidgetEmptyStateProps,
  WidgetEmptyStateTextProps,
  WidgetEmptyStateTitleProps,
  WidgetSubtitleProps,
} from "./widget-frame-content";
export {
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_WIDE_BODY_CLASS,
  widgetFrameDetailCardClass,
} from "./widget-frame-layout";
export { WidgetFrame } from "./widget-frame";
export type { WidgetFrameProps } from "./widget-frame";
export {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WIDGET_FRAME_SUPPORTING_LABEL_CLASS,
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
} from "./widget-frame-typography";
