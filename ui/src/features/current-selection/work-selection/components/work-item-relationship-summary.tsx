import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../../../components/ui/dashboard-typography";
import type { SelectedWorkRelationshipNode } from "../lib/selected-work-relationship-graph";
import type { useCurrentSelectionDispatchHistoryMessages } from "../../base/components/current-selection-locale";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  TRACE_ACTION_LINK_CLASS,
} from "../../base/components/detail-card-shared";

export function FocusedRelationshipSummary({
  activeTraceID,
  messages,
  node,
  onSelectTraceID,
  traceTargetId,
}: {
  activeTraceID?: string | null;
  messages: ReturnType<typeof useCurrentSelectionDispatchHistoryMessages>;
  node: SelectedWorkRelationshipNode;
  onSelectTraceID?: (traceID: string) => void;
  traceTargetId: string;
}) {
  const traceID = node.traceID;

  return (
    <section
      aria-label={messages.relationshipFocusSummaryHeading}
      className="grid gap-3 rounded-xl border border-af-border bg-af-surface-raised p-3"
    >
      <div className="grid gap-1">
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
          {messages.relationshipFocusSummaryHeading}
        </span>
        <code className="min-w-0 break-words text-sm leading-5 text-af-text">
          {node.label}
        </code>
      </div>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <div>
          <dt>{messages.workIdLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            <code className={RUNTIME_DETAIL_CODE_CLASS}>{node.workID}</code>
          </dd>
        </div>
        <div>
          <dt>{messages.workTypeLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {node.workTypeID ?? messages.relationshipMetadataUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.stateLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {node.state ?? messages.relationshipMetadataUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.traceIdsLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {traceID ?? messages.relationshipMetadataUnavailable}
            {traceID && activeTraceID === traceID
              ? messages.selectedTraceSuffix
              : ""}
          </dd>
        </div>
        <div>
          <dt>{messages.relationshipRoleLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {messages.relationshipCurrentSelectionBadge}
          </dd>
        </div>
      </dl>
      {traceID && onSelectTraceID ? (
        <a
          className={TRACE_ACTION_LINK_CLASS}
          href={`#${traceTargetId}`}
          onClick={() => onSelectTraceID(traceID)}
        >
          {messages.relationshipOpenTraceAction}
        </a>
      ) : null}
    </section>
  );
}
