import type { ReactNode } from "react";

import { useState } from "react";
import {
  DASHBOARD_WIDGET_CLASS,
  DETAIL_CARD_CLASS,
  DETAIL_COPY_CLASS,
} from "../../../components/ui/widget-frame";
import {
  Collapsible,
  CollapsibleContent,
} from "../../../components/ui/collapsible";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DisclosureButton } from "../../../components/ui/disclosure-button";
import { SelectableCardButton } from "../../../components/ui/selectable-card-button";
import { cn } from "../../../lib/cn";
import { AgentBentoCard } from "../../bento/public";
import type { GraphSemanticIconKind } from "../../flowchart/public";
import { GraphSemanticIcon } from "../../flowchart/public";
import type { TerminalWorkItem, TerminalWorkStatus } from "../lib/types";
import { getTerminalWorkMessages } from "../messages/terminal-work";

export type { TerminalWorkItem, TerminalWorkStatus } from "../lib/types";

export interface CompletedFailedWorkstationCardProps {
  className?: string;
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  headerAction?: ReactNode;
  locale?: string;
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
  fallbackMessage: string;
  iconLabel: string;
  itemCountLabel: string;
  items: TerminalWorkItem[];
  onExpandedChange: (expanded: boolean) => void;
  onSelectItem: (item: TerminalWorkItem) => void;
  selectedLabel?: string;
  status: TerminalWorkStatus;
  summary: (status: TerminalWorkStatus, workstation: string) => string;
  title: string;
  toggleLabel: string;
  widgetId: string;
}

const TERMINAL_ROWS_CLASS = "grid gap-3";
const TERMINAL_ROW_CLASS =
  "grid gap-2.5 rounded-lg border border-af-border bg-af-surface-subtle p-3";
const TERMINAL_FAILED_ROW_CLASS = "border-af-danger-border";
const TERMINAL_ROW_HEADER_CLASS =
  "mb-1.5 flex flex-wrap items-start justify-between gap-2 [&_h4]:m-0 [&_p]:m-0 [&_p]:mt-1 [&_p]:text-[0.82rem] [&_p]:text-af-text-subtle";
const TERMINAL_ROW_TITLE_CLASS = "flex min-w-0 flex-1 items-center gap-2";
const TERMINAL_ROW_TITLE_ICON_CLASS = "h-4 w-4 shrink-0";
const TERMINAL_LIST_CLASS = "grid gap-2";
const TERMINAL_TOGGLE_CLASS =
  "min-h-9 shrink-0 border-af-border bg-af-surface-raised px-2.5 py-1.5 text-xs text-af-text-muted hover:border-af-border-strong hover:bg-af-overlay hover:text-af-text";
const TERMINAL_BUTTON_CLASS = cn(
  "grid h-auto min-h-0 w-full justify-start gap-1 border-af-info-border bg-af-info-surface px-2.5 py-2 text-left text-af-on-info [overflow-wrap:anywhere]",
  DASHBOARD_BODY_TEXT_CLASS,
);
const TERMINAL_BUTTON_FAILED_CLASS =
  "border-af-danger-border bg-af-danger-surface text-af-on-danger";
const TERMINAL_BUTTON_SELECTED_CLASS =
  "border-af-accent-border bg-af-accent-surface text-af-text shadow-af-accent-selected";
const TERMINAL_BUTTON_LABEL_CLASS = "font-bold";
const TERMINAL_BUTTON_META_CLASS = cn(
  "leading-snug text-current",
  DASHBOARD_SUPPORTING_TEXT_CLASS,
);

function terminalStatusIconKind(
  status: TerminalWorkStatus,
): GraphSemanticIconKind {
  return status === "failed" ? "failed" : "terminal";
}

function terminalStatusIconClassName(status: TerminalWorkStatus): string {
  return status === "failed" ? "text-af-on-danger" : "text-af-on-info";
}

export function CompletedFailedWorkstationCard({
  className = "",
  completedItems,
  failedItems,
  headerAction,
  locale,
  onSelectItem,
  selectedItem = null,
  title,
  widgetId = "terminal-work",
}: CompletedFailedWorkstationCardProps) {
  const [completedExpanded, setCompletedExpanded] = useState(true);
  const [failedExpanded, setFailedExpanded] = useState(true);
  const cardClassName = cn(
    DASHBOARD_WIDGET_CLASS,
    DETAIL_CARD_CLASS,
    className,
  );
  const messages = getTerminalWorkMessages(locale);
  const resolvedTitle = title ?? messages.cardTitle;

  return (
    <AgentBentoCard
      className={cardClassName}
      headerAction={headerAction}
      title={resolvedTitle}
    >
      <fieldset className={TERMINAL_ROWS_CLASS}>
        <legend className="sr-only">{messages.legendLabel}</legend>
        <TerminalWorkRow
          emptyMessage={messages.emptyState("completed")}
          expanded={completedExpanded}
          fallbackMessage={messages.sessionSummaryFallback("completed")}
          iconLabel={messages.iconLabel("completed")}
          itemCountLabel={messages.itemCountLabel(completedItems.length)}
          items={completedItems}
          onExpandedChange={setCompletedExpanded}
          onSelectItem={(item) => onSelectItem("completed", item)}
          selectedLabel={
            selectedItem?.status === "completed"
              ? selectedItem.label
              : undefined
          }
          toggleLabel={messages.disclosureLabel(completedExpanded)}
          status="completed"
          summary={messages.summary}
          title={messages.rowTitle("completed")}
          widgetId={widgetId}
        />
        <TerminalWorkRow
          emptyMessage={messages.emptyState("failed")}
          expanded={failedExpanded}
          fallbackMessage={messages.sessionSummaryFallback("failed")}
          iconLabel={messages.iconLabel("failed")}
          itemCountLabel={messages.itemCountLabel(failedItems.length)}
          items={failedItems}
          onExpandedChange={setFailedExpanded}
          onSelectItem={(item) => onSelectItem("failed", item)}
          selectedLabel={
            selectedItem?.status === "failed" ? selectedItem.label : undefined
          }
          toggleLabel={messages.disclosureLabel(failedExpanded)}
          status="failed"
          summary={messages.summary}
          title={messages.rowTitle("failed")}
          widgetId={widgetId}
        />
      </fieldset>
    </AgentBentoCard>
  );
}

function TerminalWorkRow({
  emptyMessage,
  expanded,
  fallbackMessage,
  iconLabel,
  itemCountLabel,
  items,
  onExpandedChange,
  onSelectItem,
  selectedLabel,
  status,
  summary,
  title,
  toggleLabel,
  widgetId,
}: TerminalWorkRowProps) {
  const rowId = `${widgetId}-${status}-items`;

  return (
    <section
      className={cn(
        TERMINAL_ROW_CLASS,
        status === "failed" && TERMINAL_FAILED_ROW_CLASS,
      )}
      aria-labelledby={`${rowId}-heading`}
      data-terminal-work-status={status}
    >
      <Collapsible onOpenChange={onExpandedChange} open={expanded}>
        <div className={TERMINAL_ROW_HEADER_CLASS}>
          <div>
            <div className={TERMINAL_ROW_TITLE_CLASS} data-terminal-work-title>
              <GraphSemanticIcon
                className={cn(
                  TERMINAL_ROW_TITLE_ICON_CLASS,
                  terminalStatusIconClassName(status),
                )}
                kind={terminalStatusIconKind(status)}
                label={iconLabel}
              />
              <h4
                className={DASHBOARD_SECTION_HEADING_CLASS}
                id={`${rowId}-heading`}
              >
                {title}
              </h4>
            </div>
            <p className={DASHBOARD_SUPPORTING_TEXT_CLASS}>{itemCountLabel}</p>
          </div>
          <DisclosureButton
            controlsID={rowId}
            expanded={expanded}
            className={TERMINAL_TOGGLE_CLASS}
            onClick={() => onExpandedChange(!expanded)}
            size="sm"
            tone="secondary"
          >
            {toggleLabel}
          </DisclosureButton>
        </div>

        <CollapsibleContent className={TERMINAL_LIST_CLASS} id={rowId}>
          {items.length > 0 ? (
            items.map((item) => (
              <SelectableCardButton
                aria-label={item.label}
                className={cn(
                  TERMINAL_BUTTON_CLASS,
                  status === "failed" && TERMINAL_BUTTON_FAILED_CLASS,
                  selectedLabel === item.label &&
                    TERMINAL_BUTTON_SELECTED_CLASS,
                )}
                data-selected={
                  selectedLabel === item.label ? "true" : undefined
                }
                key={`${status}-${item.label}`}
                onClick={() => onSelectItem(item)}
                selected={selectedLabel === item.label}
                size="sm"
                tone="outline"
              >
                <span className={TERMINAL_BUTTON_LABEL_CLASS}>
                  {item.label}
                </span>
                {renderTerminalWorkContext(
                  item,
                  fallbackMessage,
                  summary,
                  status,
                )}
              </SelectableCardButton>
            ))
          ) : (
            <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
          )}
        </CollapsibleContent>
      </Collapsible>
    </section>
  );
}

function renderTerminalWorkContext(
  item: TerminalWorkItem,
  fallbackMessage: string,
  summary: (status: TerminalWorkStatus, workstation: string) => string,
  status: TerminalWorkStatus,
) {
  const latestAttempt = item.attempts?.[item.attempts.length - 1];
  const workstation =
    item.workstationName ??
    latestAttempt?.workstation_name ??
    latestAttempt?.transition_id;
  if (!workstation) {
    return (
      <span className={TERMINAL_BUTTON_META_CLASS}>{fallbackMessage}</span>
    );
  }

  return (
    <span className={TERMINAL_BUTTON_META_CLASS}>
      {summary(status, workstation)}
    </span>
  );
}
