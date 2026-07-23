/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";
export type {
  CodePanelProps,
  DataTableColumn,
  DataTableProps,
  DataTableState,
  TableProps,
  TableSize,
} from "./data-display";
export {
  CodePanel,
  codePanelVariants,
  DataTable,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  tableCellTruncateClassName,
  tableCellWrapClassName,
  tableMinWidthWideClassName,
  tableNarrowContainerClassName,
} from "./data-display";
export type { DescriptionListProps } from "./data-display/description-list";
export { DescriptionList } from "./data-display/description-list";
export type {
  FactoryEmulatorAction,
  FactoryEmulatorControlsProps,
  FactoryEmulatorRuntimeStatus,
  FactoryEmulatorSpeed,
} from "./factory-emulator";
export { FactoryEmulatorControls } from "./factory-emulator";
export type {
  AlertPanelProps,
  AlertPanelSemanticVariant,
  AlertPanelStatusLabelProps,
  AlertPanelTextProps,
  AlertPanelTitleProps,
  AlertPanelTone,
  AlertPanelVariant,
} from "./feedback";
export {
  AlertPanel,
  AlertPanelStatusLabel,
  AlertPanelText,
  AlertPanelTitle,
  Skeleton,
} from "./feedback";
export type {
  EnumSelectOption,
  EnumSelectProps,
  FormDescriptionProps,
  FormErrorProps,
  FormFieldGroupLabelProps,
  FormFieldGroupProps,
  FormFieldMessageIds,
  FormFieldProps,
  FormHelperTextProps,
  FormLabelProps,
  FormSuccessProps,
  FormWarningProps,
  NativeSelectProps,
  OptionalEnumSelectProps,
  PackageCheckboxProps,
  PackageFileInputProps,
  PackageInputProps,
  PackageTextareaProps,
  ResetEnumSelectProps,
  SelectContentProps,
  SelectEmptyProps,
  SelectFieldProps,
  SelectItemProps,
  SelectLabelProps,
  SelectSeparatorProps,
  SelectTriggerProps,
} from "./forms";
export {
  buildFormFieldAriaDescribedBy,
  ENUM_SELECT_EMPTY_VALUE,
  EnumSelect,
  FormDescription,
  FormError,
  FormField,
  FormFieldGroup,
  FormFieldGroupLabel,
  FormHelperText,
  FormLabel,
  FormSuccess,
  FormWarning,
  inputVariants,
  NativeSelect,
  OptionalEnumSelect,
  PackageCheckbox,
  PackageFileInput,
  PackageInput,
  PackageTextarea,
  ResetEnumSelect,
  SELECT_EMPTY_STATE_VALUE,
  Select,
  SelectContent,
  SelectEmpty,
  SelectField,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
  textareaVariants,
} from "./forms";
export type { ActionRowProps } from "./layout/action-row";
export { ActionRow } from "./layout/action-row";
export type { SurfacePanelProps } from "./layout/surface-panel";
export { SurfacePanel, surfacePanelVariants } from "./layout/surface-panel";
export type {
  DialogContentProps,
  ScrollAreaProps,
  ScrollBarProps,
} from "./overlays";
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
  ButtonLinkProps,
  ButtonProps,
  IconButtonShellProps,
} from "./primitives";
export {
  Button,
  ButtonLink,
  buttonVariants,
  IconButtonShell,
} from "./primitives";
export type {
  CodeProps,
  HeadingProps,
  LabelProps,
  TextProps,
  TextVariant,
} from "./primitives/typography";
export { Code, Heading, Label, Text } from "./primitives/typography";
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
  widgetFrameDetailCardClass,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "./recipes";
