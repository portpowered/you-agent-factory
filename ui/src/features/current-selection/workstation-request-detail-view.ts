import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { formatWorkItemLabel } from "../../components/ui/formatters";
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
  requestTitle: string;
  requestRunner: DashboardSelectedRunner | undefined;
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
    requestTitle: workstationRequestTitle(request),
    requestRunner: request.request_view?.runner,
    totalDurationMillis:
      request.total_duration_millis ?? request.script_response?.duration_millis,
  };
}

function workstationRequestTitle(
  request: WorkstationRequestDetailCardProps["request"],
): string {
  const namedInputWorkItem = request.request_view?.input_work_items?.find(
    hasWorkFacingLabel,
  );
  if (namedInputWorkItem) {
    return formatWorkItemLabel(namedInputWorkItem);
  }

  const namedWorkItem = request.work_items.find(hasWorkFacingLabel);
  if (namedWorkItem) {
    return formatWorkItemLabel(namedWorkItem);
  }

  return request.request_id || request.dispatch_id;
}

function hasWorkFacingLabel(workItem: { display_name?: string; work_id: string }) {
  return Boolean(workItem.display_name?.trim() || workItem.work_id.trim());
}
