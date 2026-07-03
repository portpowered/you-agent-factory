/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";

export { DescriptionList } from "./data-display/description-list";
export type { DescriptionListProps } from "./data-display/description-list";

export {
  AlertPanel,
  AlertPanelStatusLabel,
  AlertPanelText,
  AlertPanelTitle,
  Skeleton,
} from "./feedback";
export type {
  AlertPanelProps,
  AlertPanelSemanticVariant,
  AlertPanelStatusLabelProps,
  AlertPanelTextProps,
  AlertPanelTitleProps,
  AlertPanelTone,
  AlertPanelVariant,
} from "./feedback";
export { CodePanel, codePanelVariants } from "./data-display";
export type { CodePanelProps } from "./data-display";
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
  SELECT_EMPTY_STATE_VALUE,
  SelectContent,
  SelectEmpty,
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
  SelectEmptyProps,
  SelectFieldProps,
  SelectItemProps,
  SelectLabelProps,
  SelectSeparatorProps,
  SelectTriggerProps,
} from "./forms";

export { ActionRow } from "./layout/action-row";
export type { ActionRowProps } from "./layout/action-row";
export { SurfacePanel, surfacePanelVariants } from "./layout/surface-panel";
export type { SurfacePanelProps } from "./layout/surface-panel";

export { Code, Heading, Label, Text } from "./primitives/typography";
export type {
  CodeProps,
  HeadingProps,
  LabelProps,
  TextProps,
  TextVariant,
} from "./primitives/typography";
export {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
  OVERLAY_DIALOG_BODY_CLASS,
  OVERLAY_DIALOG_CONTENT_SHELL_CLASS,
  OVERLAY_FORM_GROUP_CLASS,
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
  ScrollBar,
} from "./overlays";
export type {
  DialogContentProps,
  ScrollAreaProps,
  ScrollBarProps,
} from "./overlays";
