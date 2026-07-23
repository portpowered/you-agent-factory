/** Stable category path for `@you-agent-factory/components/recipes`. */
export const COMPONENTS_CATEGORY = "recipes" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export type { WidgetFrameProps } from "./widget-frame";
export { WidgetFrame } from "./widget-frame";
export type {
  WidgetDetailCopyProps,
  WidgetEmptyStateProps,
  WidgetEmptyStateTextProps,
  WidgetEmptyStateTitleProps,
  WidgetSubtitleProps,
} from "./widget-frame-content";
export {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetSubtitle,
} from "./widget-frame-content";
export type {
  WidgetFrameDisclosureIconProps,
  WidgetFrameDisclosurePanelProps,
  WidgetFrameDisclosureProps,
  WidgetFrameDisclosureTriggerProps,
} from "./widget-frame-disclosure";
export {
  WidgetFrameDisclosure,
  WidgetFrameDisclosureIcon,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
} from "./widget-frame-disclosure";
export {
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
  WIDGET_FRAME_RESPONSIVE_SHELL_CLASS,
  WIDGET_FRAME_STORY_SHELL_DATA_ATTR,
  WIDGET_FRAME_WIDE_BODY_CLASS,
  widgetFrameDetailCardClass,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "./widget-frame-layout";
export { WidgetFrameSkeleton } from "./widget-frame-skeleton";
export type {
  WidgetErrorStateProps,
  WidgetLoadingStateProps,
  WidgetSuccessStateProps,
} from "./widget-frame-states";
export {
  WidgetErrorState,
  WidgetLoadingState,
  WidgetSuccessState,
} from "./widget-frame-states";
export {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WIDGET_FRAME_SUPPORTING_LABEL_CLASS,
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
} from "./widget-frame-typography";
