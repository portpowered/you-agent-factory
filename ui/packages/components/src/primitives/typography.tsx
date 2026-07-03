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

type TypographyElementProps = HTMLAttributes<HTMLElement> & {
  as?: ElementType;
  children?: ReactNode;
  dateTime?: string;
  htmlFor?: string;
  type?: "button" | "reset" | "submit";
};

type TypographyOverflowProps = {
  truncate?: boolean;
  wrap?: boolean;
};

function typographyOverflowClass({
  truncate,
  wrap,
}: TypographyOverflowProps): string | undefined {
  if (truncate) {
    return TEXT_TRUNCATE_CLASS;
  }

  if (wrap) {
    return TEXT_WRAP_CLASS;
  }

  return undefined;
}

const TEXT_VARIANT_CLASS = {
  body: BODY_TEXT_CLASS,
  supporting: SUPPORTING_TEXT_CLASS,
  muted: MUTED_TEXT_CLASS,
  caption: CAPTION_TEXT_CLASS,
  dense: DENSE_BODY_TEXT_CLASS,
} as const;

export type TextVariant = keyof typeof TEXT_VARIANT_CLASS;

export interface TextProps
  extends TypographyElementProps,
    TypographyOverflowProps {
  variant?: TextVariant;
}

export const Text = forwardRef<HTMLElement, TextProps>(function Text(
  {
    as: Component = "p",
    children,
    className,
    truncate,
    variant = "body",
    wrap,
    ...props
  },
  ref,
) {
  return (
    <Component
      className={cn(
        TEXT_VARIANT_CLASS[variant],
        typographyOverflowClass({ truncate, wrap }),
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});

export interface HeadingProps
  extends TypographyElementProps,
    TypographyOverflowProps {
  level?: "page" | "section";
}

export const Heading = forwardRef<HTMLElement, HeadingProps>(function Heading(
  { as, children, className, level = "section", truncate, wrap, ...props },
  ref,
) {
  const Component = as ?? (level === "page" ? "h1" : "h3");

  return (
    <Component
      className={cn(
        level === "page" ? PAGE_HEADING_CLASS : SECTION_HEADING_CLASS,
        typographyOverflowClass({ truncate, wrap }),
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});

export interface LabelProps
  extends TypographyElementProps,
    TypographyOverflowProps {}

export const Label = forwardRef<HTMLElement, LabelProps>(function Label(
  { as: Component = "span", children, className, truncate, wrap, ...props },
  ref,
) {
  return (
    <Component
      className={cn(
        SUPPORTING_LABEL_CLASS,
        typographyOverflowClass({ truncate, wrap }),
        className,
      )}
      ref={ref}
      {...props}
    >
      {children}
    </Component>
  );
});

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
