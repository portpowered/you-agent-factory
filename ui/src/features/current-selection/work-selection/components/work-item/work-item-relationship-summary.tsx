import {
  Code,
  Label,
  surfacePanelVariants,
} from "../../../../../components/ui";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../../base/components/presentation/current-selection-locale";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
  CurrentSelectionDetailValue,
  CurrentSelectionTraceButton,
} from "../../../base/public";
import type { SelectedWorkRelationshipNode } from "../../lib/selected-work-relationship-graph";

export function FocusedRelationshipSummary({
  activeTraceID,
  messages,
  node,
  onSelectTraceID,
}: {
  activeTraceID?: string | null;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  node: SelectedWorkRelationshipNode;
  onSelectTraceID?: (traceID: string) => void;
}) {
  const traceID = node.traceID;

  return (
    <section
      aria-label={messages.relationshipFocusSummaryHeading}
      className={surfacePanelVariants({
        className: "grid gap-3",
      })}
    >
      <div className="grid gap-1">
        <Label>{messages.relationshipFocusSummaryHeading}</Label>
        <Code className="min-w-0 break-words text-sm leading-5 text-on-surface">
          {node.label}
        </Code>
      </div>
      <CurrentSelectionDescriptionList>
        <CurrentSelectionDetailItem
          code
          label={messages.workIdLabel}
          value={node.workID}
        />
        <CurrentSelectionDetailItem
          label={messages.workTypeLabel}
          value={node.workTypeID ?? messages.relationshipMetadataUnavailable}
        />
        <CurrentSelectionDetailItem
          label={messages.stateLabel}
          value={node.state ?? messages.relationshipMetadataUnavailable}
        />
        <div>
          <dt>{messages.traceIdsLabel}</dt>
          <CurrentSelectionDetailValue>
            {traceID ?? messages.relationshipMetadataUnavailable}
            {traceID && activeTraceID === traceID
              ? messages.selectedTraceSuffix
              : ""}
          </CurrentSelectionDetailValue>
        </div>
        <CurrentSelectionDetailItem
          label={messages.relationshipRoleLabel}
          value={messages.relationshipCurrentSelectionBadge}
        />
      </CurrentSelectionDescriptionList>
      {traceID && onSelectTraceID ? (
        <CurrentSelectionTraceButton
          activeTraceID={activeTraceID}
          onSelectTraceID={onSelectTraceID}
          traceID={traceID}
        >
          {messages.relationshipOpenTraceAction}
        </CurrentSelectionTraceButton>
      ) : null}
    </section>
  );
}
