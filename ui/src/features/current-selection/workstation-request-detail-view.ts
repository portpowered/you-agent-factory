import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { normalizeDetailText } from "./detail-card-shared";
import type { DashboardSelectedRunner } from "../../api/dashboard/types";

export interface WorkstationRequestDetailView {
  hasFailureDetails: boolean;
  hasFailedOutcome: boolean;
  isScriptBackedRequest: boolean;
  normalizedFailureMessage: string | undefined;
  normalizedFailureReason: string | undefined;
  normalizedScriptStderr: string | undefined;
  normalizedScriptStdout: string | undefined;
  outcome: string | undefined;
  requestRunner: DashboardSelectedRunner | undefined;
  responseRunner: DashboardSelectedRunner | undefined;
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
    requestRunner: request.request_view?.runner,
    responseRunner: request.response_view?.runner,
    totalDurationMillis:
      request.total_duration_millis ?? request.script_response?.duration_millis,
  };
}
