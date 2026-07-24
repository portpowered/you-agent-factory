/** Stable category path for `@you-agent-factory/components/overlays`. */
export const COMPONENTS_CATEGORY = "overlays" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "./collapsible";
export type { DialogContentProps } from "./dialog";
export {
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
} from "./dialog";
export {
  OVERLAY_DIALOG_BODY_CLASS,
  OVERLAY_DIALOG_CONTENT_SHELL_CLASS,
  OVERLAY_FORM_GROUP_CLASS,
} from "./overlay-layout";
export {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverTrigger,
} from "./popover";
export type { ScrollAreaProps, ScrollBarProps } from "./scroll-area";
export { ScrollArea, ScrollBar } from "./scroll-area";
