/** Stable category path for `@you-agent-factory/components/primitives`. */
export const COMPONENTS_CATEGORY = "primitives" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export type { ButtonProps } from "./button";
export { Button, buttonVariants } from "./button";
export type { ButtonLinkProps } from "./button-link";
export { ButtonLink } from "./button-link";
export type { IconButtonShellProps } from "./icon-button-shell";
export { IconButtonShell } from "./icon-button-shell";
export type { PackageTextProps, PackageTextVariant } from "./package-text";
export { PackageText } from "./package-text";
export type {
  CodeProps,
  HeadingProps,
  LabelProps,
  TextProps,
  TextVariant,
} from "./typography";
export { Code, Heading, Label, Text } from "./typography";

export {
  BODY_CODE_CLASS,
  BODY_TEXT_CLASS,
  CAPTION_TEXT_CLASS,
  DENSE_BODY_TEXT_CLASS,
  MUTED_TEXT_CLASS,
  PAGE_HEADING_CLASS,
  SECTION_HEADING_CLASS,
  SUPPORTING_CODE_CLASS,
  SUPPORTING_LABEL_CLASS,
  SUPPORTING_TEXT_CLASS,
  TEXT_TRUNCATE_CLASS,
  TEXT_WRAP_CLASS,
} from "./typography-roles";
