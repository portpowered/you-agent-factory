import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { normalizeDetailText } from "./detail-card-shared";

export interface WorkstationRequestDetailView {
  hasFailureDetails: boolean;
  hasFailedOutcome: boolean;
  isScriptBackedRequest: boolean;
  normalizedFailureMessage: string | undefined;
  normalizedFailureReason: string | undefined;
  normalizedScriptStderr: string | undefined;
  normalizedScriptStdout: string | undefined;
  outcome: string | undefined;
  totalDurationMillis: number | undefined;
}

export function buildWorkstationRequestDetailView(
  request: WorkstationRequestDetailCardProps["request"],
): WorkstationRequestDetailView {
  const isScriptBackedRequest =
    request.script_request !== undefined ||
    request.script_response !== undefined;
  const normalizedFailureReason = normalizeDetailText(request.failure_reason);
  const normalizedFailureMessage = normalizeDetailText(request.failure_message);
  const hasFailureDetails =
    normalizedFailureReason !== undefined ||
    normalizedFailureMessage !== undefined;
  const normalizedOutcome = (request.outcome ?? request.script_response?.outcome)
    ?.trim()
    .toUpperCase();
  const hasFailedOutcome =
    hasFailureDetails ||
    normalizedOutcome === "FAILED" ||
    normalizedOutcome === "FAILED_EXIT_CODE" ||
    normalizedOutcome === "TIMED_OUT" ||
    normalizedOutcome === "REJECTED";
  return {
    hasFailureDetails,
    hasFailedOutcome,
    isScriptBackedRequest,
    normalizedFailureMessage,
    normalizedFailureReason,
    normalizedScriptStderr: normalizeDetailText(
      request.script_response?.stderr,
    ),
    normalizedScriptStdout: normalizeDetailText(
      request.script_response?.stdout,
    ),
    outcome: request.outcome ?? request.script_response?.outcome,
    totalDurationMillis:
      request.total_duration_millis ?? request.script_response?.duration_millis,
  };
}
