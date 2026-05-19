import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { normalizeDetailText } from "./detail-card-shared";

export interface WorkstationRequestDetailView {
  hasFailureDetails: boolean;
  inferenceRequestDetailsCopy: string;
  inferenceResponseDetailsCopy: string;
  isScriptBackedRequest: boolean;
  normalizedFailureMessage: string | undefined;
  normalizedFailureReason: string | undefined;
  normalizedScriptStderr: string | undefined;
  normalizedScriptStdout: string | undefined;
  outcome: string | undefined;
  responseMetadataUnavailableCopy: string;
  scriptResponseUnavailableCopy: string;
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
  const hasErroredRequest =
    request.errored_request_count > 0 || hasFailureDetails;

  return {
    hasFailureDetails,
    inferenceRequestDetailsCopy:
      "Prompt, request payload, working-directory, and worktree details are shown under Inference attempts when available.",
    inferenceResponseDetailsCopy:
      "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
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
    responseMetadataUnavailableCopy: hasErroredRequest
      ? "Response metadata is unavailable because this workstation request ended with an error."
      : isScriptBackedRequest
        ? "Response metadata is not available for this script-backed workstation request."
        : "Response metadata is not available for this workstation request yet.",
    scriptResponseUnavailableCopy: hasErroredRequest
      ? "Script response details are unavailable because this workstation request ended with an error."
      : "Script response details are not available for this workstation request yet.",
    totalDurationMillis:
      request.total_duration_millis ?? request.script_response?.duration_millis,
  };
}
