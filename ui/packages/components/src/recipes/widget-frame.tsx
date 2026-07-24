import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../utilities/cn";
import {
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_WIDE_BODY_CLASS,
  widgetFrameDetailCardClass,
} from "./widget-frame-layout";
import { WIDGET_FRAME_BODY_TEXT_CLASS } from "./widget-frame-typography";

const WIDGET_FRAME_SHELL_CLASS =
  "flex min-w-0 flex-col rounded-lg border border-outline bg-surface-container-high text-on-surface shadow-af-card";
const WIDGET_FRAME_SCROLL_CLASS = "overflow-hidden";
const WIDGET_FRAME_HEADER_CLASS =
  "relative z-10 flex min-h-13 shrink-0 items-center justify-between gap-3 border-b border-outline bg-surface-container-high px-3.5 py-3";
const WIDGET_FRAME_HEADER_TOOLS_CLASS =
  "flex min-w-0 shrink-0 items-center gap-2";
const WIDGET_FRAME_BODY_SCROLL_CLASS = "min-h-0 flex-1 overflow-auto";
const WIDGET_FRAME_BODY_CLASS = cn(
  "grid min-h-0 gap-2.5 px-3.5 pt-3.5 pb-4 [&>*]:pb-1 [&_p]:m-0",
  WIDGET_FRAME_BODY_TEXT_CLASS,
);

export interface WidgetFrameProps {
  bodyClassName?: string;
  bodyProps?: HTMLAttributes<HTMLDivElement>;
  bodyScroll?: boolean;
  children: ReactNode;
  className?: string;
  headerAction?: ReactNode;
  title: string;
  wide?: boolean;
}

export function WidgetFrame({
  bodyClassName,
  bodyProps,
  bodyScroll = true,
  children,
  className,
  headerAction,
  title,
  wide = false,
}: WidgetFrameProps) {
  const cardClassName = cn(
    WIDGET_FRAME_SHELL_CLASS,
    WIDGET_FRAME_MIN_WIDTH_CLASS,
    widgetFrameDetailCardClass,
    wide && WIDGET_FRAME_WIDE_BODY_CLASS,
    bodyScroll && WIDGET_FRAME_SCROLL_CLASS,
    className,
  );
  const cardBodyClassName = cn(WIDGET_FRAME_BODY_CLASS, bodyClassName);
  const bodyClassNames = cn(
    "min-h-0 flex-1",
    bodyScroll && WIDGET_FRAME_BODY_SCROLL_CLASS,
    cardBodyClassName,
  );

  return (
    <article aria-label={title} className={cardClassName}>
      <header className={WIDGET_FRAME_HEADER_CLASS}>
        <div className="min-w-0 flex-1">
          <h3 className="m-0 min-w-0 flex-1 text-title-large uppercase text-on-surface [overflow-wrap:anywhere]">
            {title}
          </h3>
        </div>
        <div className={WIDGET_FRAME_HEADER_TOOLS_CLASS}>
          {headerAction ?? (
            <span
              aria-hidden="true"
              className="block h-10 w-10 shrink-0"
              data-widget-frame-header-action-spacer="true"
            />
          )}
        </div>
      </header>
      <div className={bodyClassNames} {...bodyProps}>
        {children}
      </div>
    </article>
  );
}
