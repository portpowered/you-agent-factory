/** Stable category path for `@you-agent-factory/components/primitives`. */
export const COMPONENTS_CATEGORY = "primitives" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export { Button, buttonVariants } from "./button";
export type { ButtonProps } from "./button";
export { ButtonLink } from "./button-link";
export type { ButtonLinkProps } from "./button-link";
export { IconButtonShell } from "./icon-button-shell";
export type { IconButtonShellProps } from "./icon-button-shell";
export { PackageText } from "./package-text";
export type { PackageTextProps, PackageTextVariant } from "./package-text";
