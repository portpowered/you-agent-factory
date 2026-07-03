/** Stable package identifier for `@you-agent-factory/components` consumers. */
export const COMPONENTS_PACKAGE_NAME = "@you-agent-factory/components" as const;

export type ComponentsPackageName = typeof COMPONENTS_PACKAGE_NAME;

export {
  COMPONENT_CATEGORY_EXPORT_PATHS,
  type ComponentCategoryExportPath,
} from "./category-paths";

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
