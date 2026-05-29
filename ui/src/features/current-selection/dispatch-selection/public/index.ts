export { WorkstationRequestDetailCard } from "../components/workstation-request-detail";
export { SelectedWorkDispatchHistorySection } from "../components/selected-work-dispatch-history";

export {
  dedupeWorkItems,
  hasResponseDetails,
  isProjectedWorkstationRequest,
  requestDurationMillis,
  requestErrorClass,
  requestFailureMessage,
  requestFailureReason,
  requestInferenceAttempts,
  requestInputWorkItems,
  requestModel,
  requestOutcome,
  requestOutputWorkItems,
  requestPrompt,
  requestProvider,
  requestProviderSession,
  requestResponseText,
  requestScriptRequest,
  requestScriptResponse,
  requestStartedAt,
  requestTitle,
  requestTraceIDs,
  requestWorkingDirectory,
  requestWorktree,
  scriptAttemptNumber,
  scriptRequestID,
  scriptResponseDurationMillis,
  scriptResponseExitCode,
  scriptResponseFailureType,
} from "../dispatch-history/selected-work-dispatch-history-helpers";

export type {
  SelectedWorkDispatchHistorySectionProps,
  SelectedWorkRequestHistoryItem,
  WorkstationRequestDetailCardProps,
} from "../lib/detail-card-types";
