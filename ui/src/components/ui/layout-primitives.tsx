import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../../lib/cn";
import {
  LAYOUT_CARD_CONTENT_STACK_CLASS,
  LAYOUT_DIALOG_BODY_CLASS,
  LAYOUT_FORM_GROUP_CLASS,
  LAYOUT_PAGE_HEADER_CLASS,
  LAYOUT_SECTION_STACK_CLASS,
  LAYOUT_TOOLBAR_ROW_CLASS,
} from "./dashboard-layout";

type LayoutPrimitiveProps = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
};

export function SectionStack({
  children,
  className,
  ...props
}: LayoutPrimitiveProps) {
  return (
    <div
      className={cn(LAYOUT_SECTION_STACK_CLASS, className)}
      data-layout-primitive="section-stack"
      {...props}
    >
      {children}
    </div>
  );
}

export function CardContentStack({
  children,
  className,
  ...props
}: LayoutPrimitiveProps) {
  return (
    <div
      className={cn(LAYOUT_CARD_CONTENT_STACK_CLASS, className)}
      data-layout-primitive="card-content"
      {...props}
    >
      {children}
    </div>
  );
}

export function PageHeaderLayout({
  actions,
  className,
  heading,
  ...props
}: Omit<HTMLAttributes<HTMLElement>, "children" | "title"> & {
  actions?: ReactNode;
  heading: ReactNode;
}) {
  return (
    <header
      className={cn(LAYOUT_PAGE_HEADER_CLASS, className)}
      data-layout-primitive="page-header"
      {...props}
    >
      <div className="grid min-w-0 gap-layout-tight">{heading}</div>
      {actions ? (
        <div className="flex shrink-0 flex-wrap items-center gap-layout-tight">
          {actions}
        </div>
      ) : null}
    </header>
  );
}

export function ToolbarRowLayout({
  children,
  className,
  ...props
}: LayoutPrimitiveProps) {
  return (
    <div
      className={cn(LAYOUT_TOOLBAR_ROW_CLASS, className)}
      data-layout-primitive="toolbar-row"
      {...props}
    >
      {children}
    </div>
  );
}

export function FormGroupLayout({
  children,
  className,
  ...props
}: LayoutPrimitiveProps) {
  return (
    <div
      className={cn(LAYOUT_FORM_GROUP_CLASS, className)}
      data-layout-primitive="form-group"
      {...props}
    >
      {children}
    </div>
  );
}

export function DialogBodyLayout({
  children,
  className,
  ...props
}: LayoutPrimitiveProps) {
  return (
    <div
      className={cn(LAYOUT_DIALOG_BODY_CLASS, className)}
      data-layout-primitive="dialog-body"
      {...props}
    >
      {children}
    </div>
  );
}
