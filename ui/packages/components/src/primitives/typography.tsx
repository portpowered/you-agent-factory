import {
  type ElementType,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { cn } from "../utilities/cn";
import {
  BODY_CODE_CLASS,
  BODY_TEXT_CLASS,
  PAGE_HEADING_CLASS,
  SECTION_HEADING_CLASS,
  SUPPORTING_CODE_CLASS,
  SUPPORTING_LABEL_CLASS,
  SUPPORTING_TEXT_CLASS,
} from "./typography-roles";

type TypographyElementProps = HTMLAttributes<HTMLElement> & {
  as?: ElementType;
  children?: ReactNode;
  dateTime?: string;
  htmlFor?: string;
  type?: "button" | "reset" | "submit";
};

export interface TextProps extends TypographyElementProps {
  variant?: "body" | "supporting";
}

export const Text = forwardRef<HTMLElement, TextProps>(function Text(
  { as: Component = "p", children, className, variant = "body", ...props },
  ref,
) {
  return (
    <Component
      className={cn(
        variant === "body" ? BODY_TEXT_CLASS : SUPPORTING_TEXT_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});

export interface HeadingProps extends TypographyElementProps {
  level?: "page" | "section";
}

export const Heading = forwardRef<HTMLElement, HeadingProps>(function Heading(
  { as, children, className, level = "section", ...props },
  ref,
) {
  const Component = as ?? (level === "page" ? "h1" : "h3");

  return (
    <Component
      className={cn(
        level === "page" ? PAGE_HEADING_CLASS : SECTION_HEADING_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});

export const Label = forwardRef<HTMLElement, TypographyElementProps>(
  function Label(
    { as: Component = "span", children, className, ...props },
    ref,
  ) {
    return (
      <Component
        className={cn(SUPPORTING_LABEL_CLASS, className)}
        ref={ref}
        {...props}
      >
        {children}
      </Component>
    );
  },
);

export interface CodeProps extends TypographyElementProps {
  size?: "body" | "supporting";
}

export const Code = forwardRef<HTMLElement, CodeProps>(function Code(
  { as: Component = "code", children, className, size = "body", ...props },
  ref,
) {
  return (
    <Component
      className={cn(
        size === "body" ? BODY_CODE_CLASS : SUPPORTING_CODE_CLASS,
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});
