import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { GridLayout, useContainerWidth } from "react-grid-layout";
import type { Layout, LayoutItem } from "react-grid-layout";
import "react-grid-layout/css/styles.css";
import "react-resizable/css/styles.css";

import { cx } from "../../components/ui/classnames";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../components/ui/dashboard-typography";

export interface AgentBentoLayoutItem {
  h: number;
  id: string;
  maxH?: number;
  maxW?: number;
  minH?: number;
  minW?: number;
  w: number;
  x: number;
  y: number;
}

export interface AgentBentoLayoutCard {
  children: ReactNode;
  id: string;
}

export interface AgentBentoLayoutProps {
  cards: AgentBentoLayoutCard[];
  className?: string;
  initialWidth?: number;
  layout: AgentBentoLayoutItem[];
  onLayoutChange?: (layout: AgentBentoLayoutItem[]) => void;
}

export interface AgentBentoCardProps {
  bodyClassName?: string;
  children: ReactNode;
  className?: string;
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
const BENTO_CARD_CLASS =
  "flex h-full min-w-0 flex-col overflow-hidden rounded-lg border border-af-overlay/10 bg-af-surface/84 text-af-ink shadow-af-card";
const BENTO_CARD_HEADER_CLASS =
  "flex min-h-13 cursor-move items-center justify-between gap-3 border-af-overlay/10 px-[0.85rem] py-[0.7rem]";
const BENTO_CARD_TITLE_CLASS = cx(
  "m-0 [overflow-wrap:anywhere]",
  DASHBOARD_SECTION_HEADING_CLASS,
);
const BENTO_CARD_HEADER_TOOLS_CLASS = "flex min-w-0 shrink-0 items-center gap-2";
const BENTO_DRAG_HANDLE_CLASS =
  "inline-grid size-9 shrink-0 cursor-grab place-items-center rounded-lg border border-af-overlay/18 bg-af-overlay/8 text-af-ink/68 outline-af-ink/55 transition-colors hover:border-af-overlay/28 hover:bg-af-overlay/12 hover:text-af-ink focus-visible:outline-2 focus-visible:outline-offset-2 active:cursor-grabbing";
const BENTO_CARD_BODY_CLASS = cx(
  "grid h-full min-h-0 flex-1 gap-[0.6rem] overflow-auto p-[0.9rem] [&_p]:m-0",
  DASHBOARD_BODY_TEXT_CLASS,
);

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

function toBentoLayout(layout: Layout): AgentBentoLayoutItem[] {
  return layout.map((item: LayoutItem) => ({
    h: item.h,
    id: item.i,
    maxH: item.maxH,
    maxW: item.maxW,
    minH: item.minH,
    minW: item.minW,
    w: item.w,
    x: item.x,
    y: item.y,
  }));
}

function layoutSignature(layout: AgentBentoLayoutItem[]): string {
  return layout
    .map((item) => `${item.id}:${item.x}:${item.y}:${item.w}:${item.h}`)
    .join("|");
}

function gridLayoutSignature(layout: Layout): string {
  return layout.map((item) => `${item.i}:${item.x}:${item.y}:${item.w}:${item.h}`).join("|");
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
  onLayoutChange,
}: AgentBentoLayoutProps) {
  const normalizedLayout = useMemo(() => toGridLayout(layout), [layout]);
  const [currentLayout, setCurrentLayout] = useState<Layout>(normalizedLayout);
  const { containerRef, width } = useContainerWidth({ initialWidth });
  const renderedLayout = hasSameLayoutItems(currentLayout, normalizedLayout)
    ? currentLayout
    : normalizedLayout;

  useEffect(() => {
    setCurrentLayout(normalizedLayout);
  }, [normalizedLayout]);

  const handleLayoutChange = (nextLayout: Layout) => {
    if (gridLayoutSignature(nextLayout) === gridLayoutSignature(renderedLayout)) {
      return;
    }

    setCurrentLayout(nextLayout);
    onLayoutChange?.(toBentoLayout(nextLayout));
  };

  const layoutClassName = cx(BENTO_LAYOUT_CLASS, className);
  const renderedWidth = Math.max(width, 320);

  return (
    <section
      aria-label="Agent Factory bento board"
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
            data-bento-card-id={card.id}
            data-layout-signature={layoutSignature(toBentoLayout(currentLayout))}
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
  headerAction,
  title,
}: AgentBentoCardProps) {
  const cardClassName = cx(BENTO_CARD_CLASS, className);
  const cardBodyClassName = cx(BENTO_CARD_BODY_CLASS, bodyClassName);

  return (
    <article aria-label={title} className={cardClassName}>
      <header className={BENTO_CARD_HEADER_CLASS}>
        <h3 className={BENTO_CARD_TITLE_CLASS}>{title}</h3>
        <div className={BENTO_CARD_HEADER_TOOLS_CLASS}>
          {headerAction}
          <button
            aria-label={`Move ${title}`}
            className={BENTO_DRAG_HANDLE_CLASS}
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
        </div>
      </header>
      <div className={cardBodyClassName}>{children}</div>
    </article>
  );
}

