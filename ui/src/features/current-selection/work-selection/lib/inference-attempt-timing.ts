import type { DashboardInferenceAttempt } from "../../../../api/dashboard/types";
import {
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../../components/ui/formatters";
import type { CurrentSelectionDetailMessages } from "../../base/messages/current-selection-detail";

export function getInferenceAttemptTimingSummary(
  attempt: DashboardInferenceAttempt,
  detailMessages: CurrentSelectionDetailMessages,
  locale?: string | null,
): string | undefined {
  if (attempt.duration_millis !== undefined) {
    return `${detailMessages.elapsedTimeLabel}: ${formatDurationMillis(
      attempt.duration_millis,
      locale,
    )}`;
  }

  if (attempt.response_time) {
    return `${detailMessages.responseTimeLabel}: ${formatLocalDateTime(
      attempt.response_time,
      detailMessages.timestampUnavailable,
      locale,
    )}`;
  }

  return undefined;
}
