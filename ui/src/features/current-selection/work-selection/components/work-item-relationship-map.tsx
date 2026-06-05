import { DashboardStatusPill, SurfacePanel } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import type { CurrentSelectionDispatchHistoryMessages } from "../../base/messages/current-selection-dispatch-history";
import { CurrentSelectionSelectableButton } from "../../base/components/current-selection-selectable-button";
import { CurrentSelectionLabel } from "../../base/public";
import type { SelectedWorkRelationshipNode } from "../lib/selected-work-relationship-graph";
import {
  type RelatedWorkItem,
  relationshipDirectionGlyph,
  relationshipDirectionTone,
  relationshipLegendItems,
  relationshipPillTone,
} from "../lib/work-item-relationship-groups";

export function RelationshipLane({
  className,
  items,
  label,
  messages,
  onSelectWorkID,
}: {
  className?: string;
  items: RelatedWorkItem[];
  label: string;
  messages: CurrentSelectionDispatchHistoryMessages;
  onSelectWorkID?: (workID: string) => void;
}) {
  if (items.length === 0) {
    return null;
  }

  return (
    <SurfacePanel
      asChild
      aria-label={messages.relationshipLaneAriaLabel(label)}
      className={cn("grid gap-2", className)}
    >
      <section>
        <div className="flex items-center gap-2">
          <RelationshipDirectionBadge label={label} messages={messages} />
          <CurrentSelectionLabel>{label}</CurrentSelectionLabel>
        </div>
        <ul className="m-0 grid list-none gap-2 p-0">
          {items.map((relationship) => (
            <li key={relationship.key}>
              <SurfacePanel asChild className="grid gap-2" radius="lg">
                <article>
                  <div className="flex items-center gap-2">
                    <DashboardStatusPill
                      aria-hidden="true"
                      className="min-h-6 px-2 py-0.5 text-[11px]"
                      tone={relationshipPillTone(relationship.group)}
                    >
                      {relationshipDirectionGlyph(label, messages)}
                    </DashboardStatusPill>
                    <CurrentSelectionLabel>
                      {relationship.description}
                    </CurrentSelectionLabel>
                  </div>
                  <RelationshipNodeCard
                    heading={relationship.edgeLabel}
                    label={relationship.workLabel}
                    messages={messages}
                    node={relationship.node}
                    onSelectWorkID={onSelectWorkID}
                  />
                </article>
              </SurfacePanel>
            </li>
          ))}
        </ul>
      </section>
    </SurfacePanel>
  );
}

export function RelationshipLegend({
  messages,
}: {
  messages: CurrentSelectionDispatchHistoryMessages;
}) {
  return (
    <SurfacePanel
      asChild
      aria-label={messages.relationshipLegendHeading}
      className="grid gap-2"
      radius="lg"
    >
      <section>
        <CurrentSelectionLabel>
          {messages.relationshipLegendHeading}
        </CurrentSelectionLabel>
        <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
          {relationshipLegendItems(messages).map((item) => (
            <li key={item.label}>
              <DashboardStatusPill tone={item.tone}>
                <span aria-hidden="true">{item.glyph}</span>
                <span>{item.label}</span>
              </DashboardStatusPill>
            </li>
          ))}
        </ul>
      </section>
    </SurfacePanel>
  );
}

export function RelationshipNodeCard({
  ariaCurrent,
  className,
  heading,
  isSelected = false,
  label,
  messages,
  node,
  onSelectWorkID,
}: {
  ariaCurrent?: "true";
  className?: string;
  heading: string;
  isSelected?: boolean;
  label: string;
  messages: CurrentSelectionDispatchHistoryMessages;
  node?: SelectedWorkRelationshipNode;
  onSelectWorkID?: (workID: string) => void;
}) {
  const metadata = buildRelationshipMetadata(node, messages);

  return (
    <SurfacePanel
      aria-current={ariaCurrent}
      className={cn("grid min-w-0 gap-2", className)}
      data-selected-work-relationship-node={isSelected ? "selected" : "related"}
      tone={isSelected ? "selected" : "default"}
    >
      <div className="flex flex-wrap items-center gap-2">
        <CurrentSelectionLabel>{heading}</CurrentSelectionLabel>
        {isSelected ? (
          <DashboardStatusPill tone="active">
            {messages.relationshipCurrentSelectionBadge}
          </DashboardStatusPill>
        ) : null}
      </div>
      {node && onSelectWorkID && !isSelected ? (
        <CurrentSelectionSelectableButton
          aria-label={messages.relatedWorkSelectLabel(label)}
          className="w-full justify-start"
          onClick={() => onSelectWorkID(node.workID)}
        >
          <span className="min-w-0 break-words text-left leading-5">
            {label}
          </span>
        </CurrentSelectionSelectableButton>
      ) : (
        <code className="min-w-0 break-words text-sm leading-5 text-on-surface">
          {label}
        </code>
      )}
      <ul className="m-0 flex list-none flex-wrap gap-2 p-0">
        {metadata.map((item) => (
          <li key={item.key}>
            <DashboardStatusPill
              className="min-h-6 max-w-full gap-1 px-2 py-0.5 text-[11px]"
              tone={item.tone}
            >
              <span>{item.label}</span>
              <code className="min-w-0 break-words text-[11px] font-semibold">
                {item.value}
              </code>
            </DashboardStatusPill>
          </li>
        ))}
      </ul>
    </SurfacePanel>
  );
}

function RelationshipDirectionBadge({
  label,
  messages,
}: {
  label: string;
  messages: CurrentSelectionDispatchHistoryMessages;
}) {
  return (
    <DashboardStatusPill
      aria-hidden="true"
      className="h-8 min-h-8 w-8 px-0 py-0 text-sm font-bold"
      tone={relationshipDirectionTone(label, messages)}
      typography="none"
    >
      {relationshipDirectionGlyph(label, messages)}
    </DashboardStatusPill>
  );
}

function buildRelationshipMetadata(
  node: SelectedWorkRelationshipNode | undefined,
  messages: CurrentSelectionDispatchHistoryMessages,
) {
  return [
    {
      key: "state",
      label: messages.stateLabel,
      tone: "active" as const,
      value: node?.state ?? messages.relationshipMetadataUnavailable,
    },
    {
      key: "work-type",
      label: messages.workTypeLabel,
      tone: "neutral" as const,
      value: node?.workTypeID ?? messages.relationshipMetadataUnavailable,
    },
    {
      key: "trace-id",
      label: messages.traceIdsLabel,
      tone: "warning" as const,
      value: node?.traceID ?? messages.relationshipMetadataUnavailable,
    },
  ];
}
