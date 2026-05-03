import { useState } from "react";

import { GraphSemanticIcon } from "../flowchart/graph-semantic-icon";
import type { GraphSemanticIconKind } from "../flowchart/graph-semantic-icon";
import { cx } from "../../lib/cx";
import {
  formatProviderSession,
  formatTraceOutcome,
} from "../../components/ui/formatters";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../components/dashboard/typography";
import {
  DASHBOARD_WIDGET_CLASS,
  DETAIL_CARD_CLASS,
  DETAIL_COPY_CLASS,
} from "../../components/dashboard/widget-board";
import { AgentBentoCard } from "../../components/ui";
import { Button } from "../../components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../components/ui/collapsible";
import type {
  DashboardProviderSessionAttempt,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

export type TerminalWorkStatus = "completed" | "failed";

export interface TerminalWorkItem {
  attempts?: DashboardProviderSessionAttempt[];
  failureMessage?: string;
  failureReason?: string;
  label: string;
  traceWorkID: string;
  workItem?: DashboardWorkItemRef;
}

export interface CompletedFailedWorkstationCardProps {
  className?: string;
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  onMove?: (widgetId: "terminal-work", direction: "left" | "right") => void;
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  order?: number;
  selectedItem?: { label: string; status: TerminalWorkStatus } | null;
  title?: string;
  widgetId?: string;
}

interface TerminalWorkRowProps {
  emptyMessage: string;
  expanded: boolean;
  items: TerminalWorkItem[];
  onExpandedChange: (expanded: boolean) => void;
  onSelectItem: (item: TerminalWorkItem) => void;
  selectedLabel?: string;
  status: TerminalWorkStatus;
  title: string;
}

const TERMINAL_ROWS_CLASS = "grid gap-[0.8rem]";
const TERMINAL_ROW_CLASS = "grid gap-3 rounded-lg border border-af-overlay/8 p-[0.85rem]";
const TERMINAL_FAILED_ROW_CLASS = "border-af-danger/50";
const TERMINAL_ROW_HEADER_CLASS =
  "mb-[0.55rem] flex items-center justify-between gap-2 [&_h4]:m-0 [&_p]:m-0 [&_p]:mt-1 [&_p]:text-[0.82rem] [&_p]:text-af-ink/58";
const TERMINAL_ROW_TITLE_CLASS = "flex min-w-0 items-center gap-2";
const TERMINAL_ROW_TITLE_ICON_CLASS = "h-4 w-4 shrink-0";
const TERMINAL_LIST_CLASS = "grid gap-2";
const TERMINAL_TOGGLE_CLASS = "min-h-9 shrink-0 px-[0.65rem] py-[0.45rem] text-xs";
const TERMINAL_BUTTON_CLASS = cx(
  "grid h-auto min-h-0 w-full justify-start gap-[0.3rem] border-af-info/35 bg-af-info/10 px-3 py-[0.55rem] text-left text-af-info-ink [overflow-wrap:anywhere]",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TERMINAL_BUTTON_FAILED_CLASS = "border-af-danger/35 bg-af-danger/10 text-af-danger-ink";
const TERMINAL_BUTTON_SELECTED_CLASS =
  "border-af-accent/55 bg-af-accent/14 text-af-accent shadow-af-accent-chip";
const TERMINAL_BUTTON_LABEL_CLASS = "font-bold";
const TERMINAL_BUTTON_META_CLASS = cx(
  "leading-snug text-af-ink/66",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);

function terminalStatusIconKind(status: TerminalWorkStatus): GraphSemanticIconKind {
  return status === "failed" ? "failed" : "terminal";
}

function terminalStatusIconClassName(status: TerminalWorkStatus): string {
  return status === "failed" ? "text-af-danger-ink/78" : "text-af-success-ink/76";
}

export function CompletedFailedWorkstationCard({
  className = "",
  completedItems,
  failedItems,
  onSelectItem,
  selectedItem = null,
  title = "Completed and failed work",
}: CompletedFailedWorkstationCardProps) {
  const [completedExpanded, setCompletedExpanded] = useState(true);
  const [failedExpanded, setFailedExpanded] = useState(true);
  const cardClassName = cx(DASHBOARD_WIDGET_CLASS, DETAIL_CARD_CLASS, className);

  return (
    <AgentBentoCard className={cardClassName} title={title}>
      <div className={TERMINAL_ROWS_CLASS} aria-label="Terminal work outcomes">
        <TerminalWorkRow
          emptyMessage="No completed work recorded yet."
          expanded={completedExpanded}
          items={completedItems}
          onExpandedChange={setCompletedExpanded}
          onSelectItem={(item) => onSelectItem("completed", item)}
          selectedLabel={selectedItem?.status === "completed" ? selectedItem.label : undefined}
          status="completed"
          title="Completed"
        />
        <TerminalWorkRow
          emptyMessage="No failed work recorded yet."
          expanded={failedExpanded}
          items={failedItems}
          onExpandedChange={setFailedExpanded}
          onSelectItem={(item) => onSelectItem("failed", item)}
          selectedLabel={selectedItem?.status === "failed" ? selectedItem.label : undefined}
          status="failed"
          title="Failed"
        />
      </div>
    </AgentBentoCard>
  );
}

function TerminalWorkRow({
  emptyMessage,
  expanded,
  items,
  onExpandedChange,
  onSelectItem,
  selectedLabel,
  status,
  title,
}: TerminalWorkRowProps) {
  const rowId = `terminal-work-${status}-items`;
  const itemCountLabel = `${items.length} ${items.length === 1 ? "item" : "items"}`;
  const iconLabel = status === "failed" ? "Failed work" : "Completed work";

  return (
    <section
      className={cx(TERMINAL_ROW_CLASS, status === "failed" && TERMINAL_FAILED_ROW_CLASS)}
      aria-labelledby={`${rowId}-heading`}
      data-terminal-work-status={status}
    >
      <Collapsible onOpenChange={onExpandedChange} open={expanded}>
        <div className={TERMINAL_ROW_HEADER_CLASS}>
          <div>
            <div className={TERMINAL_ROW_TITLE_CLASS} data-terminal-work-title>
              <GraphSemanticIcon
                className={cx(
                  TERMINAL_ROW_TITLE_ICON_CLASS,
                  terminalStatusIconClassName(status),
                )}
                kind={terminalStatusIconKind(status)}
                label={iconLabel}
              />
              <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={`${rowId}-heading`}>
                {title}
              </h4>
            </div>
            <p className={DASHBOARD_SUPPORTING_TEXT_CLASS}>{itemCountLabel}</p>
          </div>
          <CollapsibleTrigger asChild>
            <Button
              aria-controls={rowId}
              aria-expanded={expanded}
              className={TERMINAL_TOGGLE_CLASS}
              size="sm"
              tone="secondary"
            >
              {expanded ? "Collapse" : "Expand"}
            </Button>
          </CollapsibleTrigger>
        </div>

        <CollapsibleContent className={TERMINAL_LIST_CLASS} id={rowId}>
          {items.length > 0 ? (
            items.map((item) => (
              <Button
                aria-label={item.label}
                className={cx(
                  TERMINAL_BUTTON_CLASS,
                  status === "failed" && TERMINAL_BUTTON_FAILED_CLASS,
                  selectedLabel === item.label && TERMINAL_BUTTON_SELECTED_CLASS,
                )}
                data-selected={selectedLabel === item.label ? "true" : undefined}
                key={`${status}-${item.label}`}
                onClick={() => onSelectItem(item)}
                size="sm"
                tone="outline"
              >
                <span className={TERMINAL_BUTTON_LABEL_CLASS}>{item.label}</span>
                {renderTerminalWorkContext(item, status)}
              </Button>
            ))
          ) : (
            <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
          )}
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}

function renderTerminalWorkContext(item: TerminalWorkItem, status: TerminalWorkStatus) {
  const latestAttempt = item.attempts?.[item.attempts.length - 1];
  if (!latestAttempt) {
    return (
      <span className={TERMINAL_BUTTON_META_CLASS}>
        {status === "failed" ? "Failed status recorded by session summary." : "Completed by session summary."}
      </span>
    );
  }

  const workstation = latestAttempt.workstation_name || latestAttempt.transition_id;
  const providerSession = formatProviderSession(latestAttempt.provider_session);

  return (
    <span className={TERMINAL_BUTTON_META_CLASS}>
      {formatTraceOutcome(latestAttempt.outcome)} at {workstation}; {providerSession}
    </span>
  );
}
