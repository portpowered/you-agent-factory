import type { ReactNode } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  StandardListSelection,
  StandardListSelectionItem,
} from "../../../components/ui/standard-list-selection";
import {
  DASHBOARD_WIDGET_CLASS,
  DETAIL_CARD_CLASS,
  DETAIL_COPY_CLASS,
} from "../../../components/ui/widget-frame";
import { cn } from "../../../lib/cn";
import { AgentBentoCard } from "../../bento/public";
import type { GraphSemanticIconKind } from "../../flowchart/public";
import { GraphSemanticIcon } from "../../flowchart/public";
import { StandardExpandableSection } from "../../standard-card-components/public";
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
  fallbackMessage: string;
  iconLabel: string;
  itemCountLabel: string;
  items: TerminalWorkItem[];
  onSelectItem: (item: TerminalWorkItem) => void;
  selectedLabel?: string;
  status: TerminalWorkStatus;
  summary: (status: TerminalWorkStatus, workstation: string) => string;
  title: string;
  toggleLabel: (expanded: boolean) => string;
  widgetId: string;
}

const TERMINAL_ROW_TITLE_ICON_CLASS = "h-4 w-4 shrink-0";
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
  return status === "failed" ? "text-on-error" : "text-on-info";
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
      <fieldset className="grid gap-3">
        <legend className="sr-only">{messages.legendLabel}</legend>
        <TerminalWorkRow
          emptyMessage={messages.emptyState("completed")}
          fallbackMessage={messages.sessionSummaryFallback("completed")}
          iconLabel={messages.iconLabel("completed")}
          itemCountLabel={messages.itemCountLabel(completedItems.length)}
          items={completedItems}
          onSelectItem={(item) => onSelectItem("completed", item)}
          selectedLabel={
            selectedItem?.status === "completed"
              ? selectedItem.label
              : undefined
          }
          toggleLabel={messages.disclosureLabel}
          status="completed"
          summary={messages.summary}
          title={messages.rowTitle("completed")}
          widgetId={widgetId}
        />
        <TerminalWorkRow
          emptyMessage={messages.emptyState("failed")}
          fallbackMessage={messages.sessionSummaryFallback("failed")}
          iconLabel={messages.iconLabel("failed")}
          itemCountLabel={messages.itemCountLabel(failedItems.length)}
          items={failedItems}
          onSelectItem={(item) => onSelectItem("failed", item)}
          selectedLabel={
            selectedItem?.status === "failed" ? selectedItem.label : undefined
          }
          toggleLabel={messages.disclosureLabel}
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
  fallbackMessage,
  iconLabel,
  itemCountLabel,
  items,
  onSelectItem,
  selectedLabel,
  status,
  summary,
  title,
  toggleLabel,
  widgetId,
}: TerminalWorkRowProps) {
  const rowId = `${widgetId}-${status}-items`;
  const headingId = `${rowId}-heading`;

  return (
    <StandardExpandableSection
      contentClassName="rounded-2xlbg-surface-container-high"
      contentID={rowId}
      defaultExpanded
      heading={title}
      headingGroupAttributes={{ "data-terminal-work-title": true }}
      headingID={headingId}
      leadingVisual={
        <GraphSemanticIcon
          className={cn(
            TERMINAL_ROW_TITLE_ICON_CLASS,
            terminalStatusIconClassName(status),
          )}
          kind={terminalStatusIconKind(status)}
          label={iconLabel}
        />
      }
      sectionAttributes={{ "data-terminal-work-status": status }}
      supportingText={itemCountLabel}
      toggleLabel={({ expanded }) => toggleLabel(expanded)}
    >
      {items.length > 0 ? (
        <StandardListSelection>
          {items.map((item) => (
            <StandardListSelectionItem
              aria-label={item.label}
              className={cn(
                "px-2.5 py-2 [overflow-wrap:anywhere]",
                DASHBOARD_BODY_TEXT_CLASS,
              )}
              key={`${status}-${item.label}`}
              onClick={() => onSelectItem(item)}
              selected={selectedLabel === item.label}
              tone={status === "failed" ? "danger" : "success"}
            >
              <span className="font-bold">{item.label}</span>
              {renderTerminalWorkContext(
                item,
                fallbackMessage,
                summary,
                status,
              )}
            </StandardListSelectionItem>
          ))}
        </StandardListSelection>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
      )}
    </StandardExpandableSection>
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
