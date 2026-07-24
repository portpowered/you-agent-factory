import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import {
  DashboardActionButton,
  surfacePanelVariants,
} from "../../../components/ui";
import { cn } from "../../../lib/cn";
import { DashboardWidgetFrame } from "../../bento/public";
import { CurrentSelectionExecutionPill } from "../../current-selection/base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../current-selection/base/components/presentation/current-selection-supporting-text";
import { WorkstationDispatchRow } from "../../current-selection/workstation-selection/components/detail-card/workstation-dispatch-row";
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
  messages: ReturnType<typeof getTerminalWorkMessages>;
  onSelectItem: (item: TerminalWorkItem) => void;
  selectedLabel?: string;
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
  const messages = getTerminalWorkMessages(locale);
  const resolvedTitle = title ?? messages.cardTitle;

  return (
    <DashboardWidgetFrame
      className={className}
      headerAction={headerAction}
      title={resolvedTitle}
      widgetId={widgetId}
    >
      <fieldset className="grid gap-3">
        <legend className="sr-only">{messages.legendLabel}</legend>
        <TerminalWorkRow
          emptyMessage={messages.emptyState("completed")}
          fallbackMessage={messages.sessionSummaryFallback("completed")}
          iconLabel={messages.iconLabel("completed")}
          itemCountLabel={messages.itemCountLabel(completedItems.length)}
          items={completedItems}
          messages={messages}
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
          messages={messages}
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
              key={`${status}-${item.label}`}
              messages={messages}
              onClick={() => onSelectItem(item)}
              selected={selectedLabel === item.label}
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
          tone={status === "failed" ? "danger" : "success"}
        >
          {messages.rowTitle(status)}
        </CurrentSelectionExecutionPill>
      }
      supportingContent={
        <div className="grid gap-1">
          {renderTerminalWorkContext(item, fallbackMessage, summary, status)}
          {selected ? (
            <CurrentSelectionSupportingText tone="status">
              {messages.selectedWorkItemLabel(item.label)}
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
