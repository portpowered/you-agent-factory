import {
  WIDGET_FRAME_MIN_WIDTH_CLASS,
  WIDGET_FRAME_WIDE_BODY_CLASS,
  widgetFrameDetailCardClass,
} from "@you-agent-factory/components/recipes";
import type { HTMLAttributes, ReactNode } from "react";

import { cn } from "../../../../lib/cn";
import { AgentBentoCard } from "../agent-bento";

export interface DashboardWidgetFrameProps {
  bodyClassName?: string;
  bodyProps?: HTMLAttributes<HTMLDivElement>;
  bodyScroll?: boolean;
  children: ReactNode;
  className?: string;
  headerAction?: ReactNode;
  title: string;
  wide?: boolean;
  widgetId: string;
}

export function DashboardWidgetFrame({
  bodyClassName,
  bodyProps,
  bodyScroll = true,
  children,
  className = "",
  headerAction,
  title,
  wide = false,
}: DashboardWidgetFrameProps) {
  return (
    <AgentBentoCard
      className={cn(
        WIDGET_FRAME_MIN_WIDTH_CLASS,
        widgetFrameDetailCardClass,
        wide && WIDGET_FRAME_WIDE_BODY_CLASS,
        className,
      )}
      bodyClassName={bodyClassName}
      bodyProps={bodyProps}
      bodyScroll={bodyScroll}
      headerAction={headerAction}
      title={title}
    >
      {children}
    </AgentBentoCard>
  );
}
