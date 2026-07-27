import type { DashboardInferenceAttempt } from "../../../../../api/dashboard/types";
import {
  formatDurationMillis,
  getLocalDateTimeDisplay,
} from "../../../../../components/ui/formatters";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import { InferenceAttemptDetail } from "./inference-attempt-detail";

export function InferenceAttemptMetadataDetails({
  attempt,
}: {
  attempt: DashboardInferenceAttempt;
}) {
  const detailMessages = useCurrentSelectionDetailMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const locale = useCurrentSelectionLocale();
  const provider =
    attempt.diagnostics?.provider?.provider ??
    attempt.provider_session?.provider;
  const model = attempt.diagnostics?.provider?.model;
  const requestTime = getLocalDateTimeDisplay(
    attempt.request_time,
    detailMessages.timestampUnavailable,
    locale,
  );
  const responseTime = getLocalDateTimeDisplay(
    attempt.response_time,
    detailMessages.timestampUnavailable,
    locale,
  );

  return (
    <CurrentSelectionDescriptionList>
      <InferenceAttemptDetail
        code
        label={detailMessages.inferenceRequestIdLabel}
        value={attempt.inference_request_id}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.providerLabel}
        value={provider}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.modelLabel}
        value={model}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.workingDirectoryLabel}
        value={attempt.working_directory}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.worktreeLabel}
        value={attempt.worktree}
      />
      <InferenceAttemptDetail
        label={detailMessages.requestTimeLabel}
        rawValue={requestTime.rawTimestamp ?? undefined}
        value={requestTime.label}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.outcomeLabel}
        value={
          attempt.outcome
            ? enumMessages.localizeOutcome(attempt.outcome)
            : undefined
        }
      />
      <InferenceAttemptDetail
        label={detailMessages.elapsedTimeLabel}
        value={
          attempt.duration_millis !== undefined
            ? formatDurationMillis(attempt.duration_millis, locale)
            : undefined
        }
      />
      <InferenceAttemptDetail
        label={detailMessages.responseTimeLabel}
        rawValue={responseTime.rawTimestamp ?? undefined}
        value={responseTime.label}
      />
      <InferenceAttemptDetail
        label={detailMessages.exitCodeLabel}
        value={attempt.exit_code}
      />
      <InferenceAttemptDetail
        code
        label={detailMessages.errorClassLabel}
        value={attempt.error_class}
      />
    </CurrentSelectionDescriptionList>
  );
}
