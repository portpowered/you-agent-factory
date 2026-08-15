import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { cn } from "../../../lib/cn";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import { CurrentSelectionExecutionPill } from "../../current-selection/base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../current-selection/base/components/presentation/current-selection-supporting-text";
import { WorkstationDispatchRow } from "../../current-selection/workstation-selection/components/detail-card/workstation-dispatch-row";
import {
  GraphSemanticIcon,
  type GraphSemanticIconKind,
} from "../../flowchart/components/graph-semantic-icon";
import { StandardExpandableSection } from "../../standard-card-components/components/standard-expandable-section";
import {
  TERMINAL_WORK_STATUSES,
  type TerminalWorkItem,
  type TerminalWorkSelection,
  type TerminalWorkStatus,
  terminalWorkIdentity,
  terminalWorkSelectionMatches,
} from "../lib/types";
import { getTerminalWorkMessages } from "../messages/terminal-work";

export type {
  TerminalWorkItem,
  TerminalWorkSelection,
  TerminalWorkStatus,
} from "../lib/types";

export interface CompletedFailedWorkstationCardProps {
  canceledItems?: TerminalWorkItem[];
  className?: string;
  completedItems: TerminalWorkItem[];
  failedItems: TerminalWorkItem[];
  headerAction?: ReactNode;
  locale?: string;
  onMove?: (widgetId: "terminal-work", direction: "left" | "right") => void;
  onSelectItem: (status: TerminalWorkStatus, item: TerminalWorkItem) => void;
  order?: number;
  selectedItem?: TerminalWorkSelection | null;
  terminatedItems?: TerminalWorkItem[];
  title?: string;
  unknownItems?: TerminalWorkItem[];
  widgetId?: string;
}

interface TerminalWorkRowProps {
  emptyMessage: string;
  fallbackMessage: string;
  iconLabel: string;
  itemCountLabel: string;
  items: TerminalWorkItem[];
  messages: ReturnType<typeof getTerminalWorkMessages>;
  onSelectItem: (item: TerminalWorkItem) => void;
  selectedItem?: TerminalWorkSelection | null;
  status: TerminalWorkStatus;
  summary: (status: TerminalWorkStatus, workstation: string) => string;
  title: string;
  toggleLabel: (expanded: boolean) => string;
  widgetId: string;
}

const TERMINAL_ROW_TITLE_ICON_CLASS = "h-4 w-4 shrink-0";
const TERMINAL_BUTTON_META_CLASS = "leading-snug text-current";

function terminalStatusIconKind(
  status: TerminalWorkStatus,
): GraphSemanticIconKind {
  switch (status) {
    case "completed":
      return "terminal";
    case "failed":
      return "failed";
    case "canceled":
      return "processing";
    case "terminated":
      return "limit";
    case "unknown":
      return "queue";
  }
}

function terminalStatusIconClassName(status: TerminalWorkStatus): string {
  switch (status) {
    case "completed":
      return "text-on-success-container";
    case "failed":
      return "text-on-error";
    case "canceled":
      return "text-on-warning-container";
    case "terminated":
      return "text-on-surface-variant";
    case "unknown":
      return "text-on-info-container";
  }
}

function terminalStatusTone(
  status: TerminalWorkStatus,
): "danger" | "info" | "neutral" | "success" | "warning" {
  switch (status) {
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "canceled":
      return "warning";
    case "terminated":
    case "unknown":
      return "neutral";
  }
}

export function CompletedFailedWorkstationCard({
  canceledItems = [],
  className = "",
  completedItems,
  failedItems,
  headerAction,
  locale,
  onSelectItem,
  selectedItem = null,
  terminatedItems = [],
  title,
  unknownItems = [],
  widgetId = "terminal-work",
}: CompletedFailedWorkstationCardProps) {
  const messages = getTerminalWorkMessages(locale);
  const resolvedTitle = title ?? messages.cardTitle;
  const itemsByStatus: Record<TerminalWorkStatus, TerminalWorkItem[]> = {
    canceled: canceledItems,
    completed: completedItems,
    failed: failedItems,
    terminated: terminatedItems,
    unknown: unknownItems,
  };

  return (
    <DashboardWidgetFrame
      className={className}
      headerAction={headerAction}
      title={resolvedTitle}
      widgetId={widgetId}
    >
      <fieldset className="grid gap-3">
        <legend className="sr-only">{messages.legendLabel}</legend>
        {TERMINAL_WORK_STATUSES.map((status) => (
          <TerminalWorkRow
            emptyMessage={messages.emptyState(status)}
            fallbackMessage={messages.sessionSummaryFallback(status)}
            iconLabel={messages.iconLabel(status)}
            itemCountLabel={messages.itemCountLabel(
              itemsByStatus[status].length,
            )}
            items={itemsByStatus[status]}
            key={status}
            messages={messages}
            onSelectItem={(item) => onSelectItem(status, item)}
            selectedItem={selectedItem}
            status={status}
            summary={messages.summary}
            title={messages.rowTitle(status)}
            toggleLabel={messages.disclosureLabel}
            widgetId={widgetId}
          />
        ))}
      </fieldset>
    </DashboardWidgetFrame>
  );
}

function TerminalWorkRow({
  emptyMessage,
  fallbackMessage,
  iconLabel,
  itemCountLabel,
  items,
  messages,
  onSelectItem,
  selectedItem,
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
      className="mt-4 gap-2.5 py-0 [&_h4]:m-0"
      contentClassName={surfacePanelVariants({
        className: "grid gap-3",
        radius: "2xl",
      })}
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
        <ul className="m-0 grid list-none gap-2.5 p-0">
          {items.map((item) => (
            <TerminalWorkListItem
              fallbackMessage={fallbackMessage}
              item={item}
              key={terminalWorkIdentity(item)}
              messages={messages}
              onClick={() => onSelectItem(item)}
              selected={terminalWorkSelectionMatches(
                item,
                selectedItem,
                status,
              )}
              selectionActionLabel={messages.selectWorkItemLabel(item.label)}
              status={status}
              summary={summary}
            />
          ))}
        </ul>
      ) : (
        <WidgetDetailCopy>{emptyMessage}</WidgetDetailCopy>
      )}
    </StandardExpandableSection>
  );
}

interface TerminalWorkListItemProps {
  fallbackMessage: string;
  item: TerminalWorkItem;
  messages: ReturnType<typeof getTerminalWorkMessages>;
  onClick: () => void;
  selected: boolean;
  selectionActionLabel: string;
  status: TerminalWorkStatus;
  summary: (status: TerminalWorkStatus, workstation: string) => string;
}

function TerminalWorkListItem({
  fallbackMessage,
  item,
  messages,
  onClick,
  selected,
  selectionActionLabel,
  status,
  summary,
}: TerminalWorkListItemProps) {
  const workID = item.workItem?.work_id ?? item.traceWorkID;

  return (
    <WorkstationDispatchRow
      actions={
        <DashboardActionButton
          aria-label={selectionActionLabel}
          aria-pressed={selected}
          onClick={onClick}
          type="button"
        >
          {selected
            ? messages.selectedWorkItemAction
            : messages.openWorkItemAction}
        </DashboardActionButton>
      }
      status={
        <CurrentSelectionExecutionPill
          aria-label={messages.rowTitle(status)}
          role="status"
          tone={terminalStatusTone(status)}
        >
          {messages.rowTitle(status)}
        </CurrentSelectionExecutionPill>
      }
      supportingContent={
        <div className="grid gap-1">
          <CurrentSelectionSupportingText
            className={TERMINAL_BUTTON_META_CLASS}
            tone="status"
          >
            {messages.workIDLabel(workID)}
          </CurrentSelectionSupportingText>
          {renderTerminalWorkContext(item, fallbackMessage, summary, status)}
          {selected ? (
            <CurrentSelectionSupportingText tone="status">
              {messages.selectedWorkItemLabel(workID)}
            </CurrentSelectionSupportingText>
          ) : null}
        </div>
      }
      title={item.label}
    />
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
      <CurrentSelectionSupportingText
        className={TERMINAL_BUTTON_META_CLASS}
        tone="status"
      >
        {fallbackMessage}
      </CurrentSelectionSupportingText>
    );
  }

  return (
    <CurrentSelectionSupportingText
      className={TERMINAL_BUTTON_META_CLASS}
      tone="status"
    >
      {summary(status, workstation)}
    </CurrentSelectionSupportingText>
  );
}
