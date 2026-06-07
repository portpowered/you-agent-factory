import { useId } from "react";
import type { DashboardInferenceAttempt } from "../../../../../api/dashboard/types";
import { DetailCopy } from "../../../../../components/ui/widget-frame";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { normalizeDetailText } from "../../../base/components/detail-card/detail-card-shared";
import { useCurrentSelectionDetailMessages } from "../../../base/components/presentation/current-selection-locale";
import { InferenceAttemptTextSection } from "./inference-attempt-text-section";

export function InferenceAttemptRequestBodySection({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();

  return (
    <InferenceAttemptBodyDisclosure
      collapseAction={detailMessages.collapseRequestBodyAction}
      expandAction={detailMessages.expandRequestBodyAction}
      label={detailMessages.requestBodyLabel}
      value={normalizeDetailText(attempt.prompt)}
    />
  );
}

export function InferenceAttemptResponseSection({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const response = normalizeDetailText(attempt.response);

  if (response) {
    return (
      <InferenceAttemptBodyDisclosure
        collapseAction={detailMessages.collapseResponseBodyAction}
        expandAction={detailMessages.expandResponseBodyAction}
        label={detailMessages.responseBodyLabel}
        value={response}
      />
    );
  }

  return attempt.outcome ? (
    <DetailCopy>{detailMessages.providerResponseUnavailable}</DetailCopy>
  ) : (
    <DetailCopy>{detailMessages.awaitingProviderResponse}</DetailCopy>
  );
}

function InferenceAttemptBodyDisclosure({
  collapseAction,
  expandAction,
  label,
  value,
}: {
  collapseAction: string;
  expandAction: string;
  label: string;
  value?: string;
}) {
  const panelId = useId();
  const labelId = `${panelId}-label`;

  if (!value) {
    return null;
  }

  return (
    <CurrentSelectionExpandableSection
      className="mt-0"
      contentId={panelId}
      headingId={labelId}
      title={label}
      toggleLabel={(expanded) => (expanded ? collapseAction : expandAction)}
    >
      <InferenceAttemptTextSection label={label} value={value} />
    </CurrentSelectionExpandableSection>
  );
}
