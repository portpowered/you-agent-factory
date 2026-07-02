import type { ComponentPropsWithoutRef } from "react";

import { cn } from "../utilities/cn";

export type PackageTextVariant = "body" | "title";

export type PackageTextProps = ComponentPropsWithoutRef<"p"> & {
  variant?: PackageTextVariant;
};

export function PackageText({
  children,
  className,
  variant = "body",
  ...props
}: PackageTextProps) {
  return (
    <p
      className={cn(
        variant === "title" ? "text-title-large" : "text-body-medium",
        "text-on-surface",
        className,
      )}
      {...props}
    >
      {children}
    </p>
  );
}
