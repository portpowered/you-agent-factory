import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import type { Layout, LayoutItem } from "react-grid-layout";
import { GridLayout, useContainerWidth } from "react-grid-layout";
import "react-grid-layout/css/styles.css";
import "react-resizable/css/styles.css";

import { cn } from "../../../lib/cn";
import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import { getAgentBentoMessages } from "../messages/agent-bento";

export interface AgentBentoLayoutItem {
  h: number;
  hidden?: boolean;
  id: string;
  maxH?: number;
  maxW?: number;
  minH?: number;
  minW?: number;
  widgetType: string;
  w: number;
  x: number;
  y: number;
}

export interface AgentBentoLayoutCard {
  children: ReactNode;
  id: string;
  widgetType: string;
}

export interface AgentBentoLayoutProps {
  cards: AgentBentoLayoutCard[];
  className?: string;
  initialWidth?: number;
  layout: AgentBentoLayoutItem[];
  locale?: string;
  onLayoutChange?: (layout: AgentBentoLayoutItem[]) => void;
}

export interface AgentBentoCardProps {
  bodyClassName?: string;
  children: ReactNode;
  className?: string;
  chromeDensity?: "compact" | "default";
  headerAction?: ReactNode;
  title: string;
}

const DEFAULT_BENTO_WIDTH = 1180;
const BENTO_COLUMNS = 12;
const BENTO_ROW_HEIGHT = 72;
const BENTO_MARGIN = [16, 16] as const;
const BENTO_CONTAINER_PADDING = [0, 0] as const;
const BENTO_RESIZE_HANDLES = ["se", "s", "e"] as const;
const BENTO_DRAG_HANDLE_SELECTOR = "[data-bento-drag-handle='true']";
const BENTO_DRAG_CANCEL_SELECTOR =
  "a,input,select,textarea,.react-resizable-handle";
const BENTO_LAYOUT_CLASS = "min-w-0 w-full";
const BENTO_GRID_CLASS = "min-h-px";
const BENTO_ITEM_CLASS = "min-w-0";
const BENTO_CARD_CLASS = "flex h-full min-w-0 flex-col overflow-hidden";
const BENTO_CARD_HEADER_CLASS =
  "flex min-h-13 cursor-move items-center justify-between gap-3 border-af-border px-3.5 py-3";
const BENTO_CARD_HEADER_COMPACT_CLASS =
  "min-h-11 gap-2 px-3 py-2.5";
const BENTO_CARD_TITLE_CLASS = cn(
  "m-0 [overflow-wrap:anywhere]",
  DASHBOARD_SECTION_HEADING_CLASS,
);
const BENTO_CARD_HEADER_TOOLS_CLASS =
  "flex min-w-0 shrink-0 items-center gap-2";
const BENTO_CARD_HEADER_TOOLS_COMPACT_CLASS = "gap-1.5";
const BENTO_DRAG_HANDLE_CLASS =
  "inline-grid size-9 shrink-0 cursor-grab place-items-center rounded-lg border border-af-border bg-af-surface-raised text-af-text-muted outline-af-accent transition-colors hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text focus-visible:outline-2 focus-visible:outline-offset-2 active:cursor-grabbing";
const BENTO_DRAG_HANDLE_COMPACT_CLASS =
  "size-8 rounded-md border-af-border bg-transparent text-af-text-subtle hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text";
const BENTO_CARD_BODY_CLASS = cn(
  "grid h-full min-h-0 flex-1 gap-2.5 overflow-auto p-3.5 [&_p]:m-0",
  DASHBOARD_BODY_TEXT_CLASS,
);
const BENTO_CARD_BODY_COMPACT_CLASS = "gap-2 p-3";

interface AgentBentoDragHandleProps {
  compact?: boolean;
  title: string;
}

function toGridLayout(layout: AgentBentoLayoutItem[]): Layout {
  return layout.map((item) => ({
    h: item.h,
    i: item.id,
    isResizable: true,
    maxH: item.maxH,
    maxW: item.maxW,
    minH: item.minH,
    minW: item.minW,
    w: item.w,
    x: item.x,
    y: item.y,
  }));
}

function toBentoLayout(
  layout: Layout,
  existingLayoutById: ReadonlyMap<string, AgentBentoLayoutItem>,
): AgentBentoLayoutItem[] {
  return layout.map((item: LayoutItem) => ({
    h: item.h,
    id: item.i,
    maxH: item.maxH,
    maxW: item.maxW,
    minH: item.minH,
    minW: item.minW,
    widgetType: existingLayoutById.get(item.i)?.widgetType ?? item.i,
    w: item.w,
    x: item.x,
    y: item.y,
  }));
}

function layoutSignature(layout: AgentBentoLayoutItem[]): string {
  return layout
    .map((item) => `${item.widgetType}:${item.x}:${item.y}:${item.w}:${item.h}`)
    .join("|");
}

function gridLayoutSignature(layout: Layout): string {
  return layout
    .map((item) => `${item.i}:${item.x}:${item.y}:${item.w}:${item.h}`)
    .join("|");
}

function hasSameLayoutItems(left: Layout, right: Layout): boolean {
  if (left.length !== right.length) {
    return false;
  }

  const rightIDs = new Set(right.map((item) => item.i));
  return left.every((item) => rightIDs.has(item.i));
}

export function AgentBentoLayout({
  cards,
  className = "",
  initialWidth = DEFAULT_BENTO_WIDTH,
  layout,
  locale,
  onLayoutChange,
}: AgentBentoLayoutProps) {
  const messages = getAgentBentoMessages(locale);
  const normalizedLayout = useMemo(() => toGridLayout(layout), [layout]);
  const layoutByID = useMemo(
    () => new Map(layout.map((item) => [item.id, item])),
    [layout],
  );
  const [currentLayout, setCurrentLayout] = useState<Layout>(normalizedLayout);
  const { containerRef, width } = useContainerWidth({ initialWidth });
  const renderedLayout = hasSameLayoutItems(currentLayout, normalizedLayout)
    ? currentLayout
    : normalizedLayout;

  useEffect(() => {
    setCurrentLayout(normalizedLayout);
  }, [normalizedLayout]);

  const handleLayoutChange = (nextLayout: Layout) => {
    if (
      gridLayoutSignature(nextLayout) === gridLayoutSignature(renderedLayout)
    ) {
      return;
    }

    setCurrentLayout(nextLayout);
    onLayoutChange?.(toBentoLayout(nextLayout, layoutByID));
  };

  const layoutClassName = cn(BENTO_LAYOUT_CLASS, className);
  const renderedWidth = Math.max(width, 320);

  return (
    <section
      aria-label={messages.boardLabel}
      className={layoutClassName}
      ref={containerRef}
    >
      <GridLayout
        autoSize
        className={BENTO_GRID_CLASS}
        dragConfig={{
          cancel: BENTO_DRAG_CANCEL_SELECTOR,
          enabled: true,
          handle: BENTO_DRAG_HANDLE_SELECTOR,
        }}
        gridConfig={{
          cols: BENTO_COLUMNS,
          containerPadding: BENTO_CONTAINER_PADDING,
          margin: BENTO_MARGIN,
          rowHeight: BENTO_ROW_HEIGHT,
        }}
        layout={renderedLayout}
        onLayoutChange={handleLayoutChange}
        resizeConfig={{ enabled: true, handles: [...BENTO_RESIZE_HANDLES] }}
        width={renderedWidth}
      >
        {cards.map((card) => (
          <div
            className={BENTO_ITEM_CLASS}
            data-bento-card-id={card.widgetType}
            data-bento-instance-id={card.id}
            data-layout-signature={layoutSignature(
              toBentoLayout(currentLayout, layoutByID).filter(
                (item) => item.id === card.id,
              ),
            )}
            id={card.id}
            key={card.id}
          >
            {card.children}
          </div>
        ))}
      </GridLayout>
    </section>
  );
}

export function AgentBentoCard({
  bodyClassName = "",
  children,
  className = "",
  chromeDensity = "default",
  headerAction,
  title,
}: AgentBentoCardProps) {
  const cardClassName = cn(BENTO_CARD_CLASS, className);
  const compactChrome = chromeDensity === "compact";
  const cardBodyClassName = cn(
    BENTO_CARD_BODY_CLASS,
    compactChrome && BENTO_CARD_BODY_COMPACT_CLASS,
    bodyClassName,
  );

  return (
    <DashboardPanelShell
      aria-label={title}
      as="article"
      className={cardClassName}
      shellKind="grid-card"
    >
      <header
        className={cn(
          BENTO_CARD_HEADER_CLASS,
          compactChrome && BENTO_CARD_HEADER_COMPACT_CLASS,
        )}
      >
        <h3 className={BENTO_CARD_TITLE_CLASS}>{title}</h3>
        <div
          className={cn(
            BENTO_CARD_HEADER_TOOLS_CLASS,
            compactChrome && BENTO_CARD_HEADER_TOOLS_COMPACT_CLASS,
          )}
        >
          {headerAction}
          <AgentBentoDragHandle compact={compactChrome} title={title} />
        </div>
      </header>
      <div className={cardBodyClassName}>{children}</div>
    </DashboardPanelShell>
  );
}

export function AgentBentoDragHandle({
  compact = false,
  title,
}: AgentBentoDragHandleProps) {
  return (
    <button
      aria-label={`Move ${title}`}
      className={cn(
        BENTO_DRAG_HANDLE_CLASS,
        compact && BENTO_DRAG_HANDLE_COMPACT_CLASS,
      )}
      data-bento-drag-handle="true"
      type="button"
    >
      <svg
        aria-hidden="true"
        fill="none"
        height="18"
        viewBox="0 0 18 18"
        width="18"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="M9 1.5v15M9 1.5 6.75 3.75M9 1.5l2.25 2.25M9 16.5l-2.25-2.25M9 16.5l2.25-2.25M1.5 9h15M1.5 9l2.25-2.25M1.5 9l2.25 2.25M16.5 9l-2.25-2.25M16.5 9l-2.25 2.25"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.7"
        />
      </svg>
    </button>
  );
}
