import { forwardRef, type ElementType, type HTMLAttributes, type ReactNode } from "react";

import { cn } from "../../lib/cn";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "./dashboard-typography";

type TypographyElementProps = HTMLAttributes<HTMLElement> & {
  as?: ElementType;
  children?: ReactNode;
  dateTime?: string;
  htmlFor?: string;
  type?: "button" | "reset" | "submit";
};

export interface DashboardTextProps extends TypographyElementProps {
  variant?: "body" | "supporting";
}

export const DashboardText = forwardRef<HTMLElement, DashboardTextProps>(
  function DashboardText(
    { as: Component = "p", children, className, variant = "body", ...props },
    ref,
  ) {
  return (
    <Component
      className={cn(
        variant === "body"
          ? DASHBOARD_BODY_TEXT_CLASS
          : DASHBOARD_SUPPORTING_TEXT_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
  },
);

export interface DashboardHeadingProps extends TypographyElementProps {
  level?: "page" | "section";
}

export const DashboardHeading = forwardRef<HTMLElement, DashboardHeadingProps>(
  function DashboardHeading(
    { as, children, className, level = "section", ...props },
    ref,
  ) {
  const Component = as ?? (level === "page" ? "h1" : "h3");

  return (
    <Component
      className={cn(
        level === "page"
          ? DASHBOARD_PAGE_HEADING_CLASS
          : DASHBOARD_SECTION_HEADING_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
  },
);

export const DashboardLabel = forwardRef<HTMLElement, TypographyElementProps>(
  function DashboardLabel(
    { as: Component = "span", children, className, ...props },
    ref,
  ) {
  return (
    <Component
      className={cn(DASHBOARD_SUPPORTING_LABEL_CLASS, className)}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
  },
);

export interface DashboardCodeProps extends TypographyElementProps {
  size?: "body" | "supporting";
}

export const DashboardCode = forwardRef<HTMLElement, DashboardCodeProps>(
  function DashboardCode(
    { as: Component = "code", children, className, size = "body", ...props },
    ref,
  ) {
  return (
    <Component
      className={cn(
        size === "body"
          ? DASHBOARD_BODY_CODE_CLASS
          : DASHBOARD_SUPPORTING_CODE_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
  },
);
