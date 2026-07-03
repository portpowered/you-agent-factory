/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";

export {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetErrorState,
  WidgetFrame,
  WidgetFrameDisclosure,
  WidgetFrameDisclosureIcon,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
  WidgetFrameSkeleton,
  WidgetLoadingState,
  WidgetSubtitle,
  WidgetSuccessState,
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_OVERFLOW_TOLERANCE_PX,
  WIDGET_FRAME_RESPONSIVE_SHELL_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_STORY_SHELL_DATA_ATTR,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WIDGET_FRAME_SUPPORTING_LABEL_CLASS,
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
  WIDGET_FRAME_WIDE_BODY_CLASS,
  widgetFrameDetailCardClass,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "./recipes";
export type {
  WidgetDetailCopyProps,
  WidgetEmptyStateProps,
  WidgetEmptyStateTextProps,
  WidgetEmptyStateTitleProps,
  WidgetErrorStateProps,
  WidgetFrameDisclosureIconProps,
  WidgetFrameDisclosurePanelProps,
  WidgetFrameDisclosureProps,
  WidgetFrameDisclosureTriggerProps,
  WidgetFrameProps,
  WidgetLoadingStateProps,
  WidgetSubtitleProps,
  WidgetSuccessStateProps,
} from "./recipes";
export {
  EnumSelect,
  ENUM_SELECT_EMPTY_VALUE,
  NativeSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
  Select,
  SelectContent,
  SelectField,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./forms";
export type {
  EnumSelectOption,
  EnumSelectProps,
  NativeSelectProps,
  OptionalEnumSelectProps,
  ResetEnumSelectProps,
  SelectContentProps,
  SelectFieldProps,
  SelectItemProps,
  SelectLabelProps,
  SelectSeparatorProps,
  SelectTriggerProps,
} from "./forms";
