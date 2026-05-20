import type { ReactNode } from "react";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type {
  PromptTemplateContract,
  PromptTemplateValidationResult,
} from "../../api/current-factory-prompt-template";
import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardPlaceRef,
  DashboardProviderSession,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import type { EditableWorkstationValues } from "../current-factory-definition/workstation-editable-values";
import type { WorkstationDetailMessages } from "./messages";
import type { CurrentSelectionDetailMessages } from "./messages/current-selection-detail";
import type { LoadableProviderSessionRef } from "./provider-session-details";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import type { DashboardWorkItemSelection, StatePositionWorkItem } from "./types";

export interface SelectionDetailLayoutProps {
  children: ReactNode;
  headerAction?: ReactNode;
  widgetId?: string;
}

export interface NoSelectionDetailCardProps {
  widgetId?: string;
}

export interface StatePositionWorkListProps {
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  messages: Pick<
    CurrentSelectionDetailMessages,
    | "failureMessageLabel"
    | "failureReasonLabel"
    | "startedAtLabel"
    | "selectWorkItemLabel"
  >;
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  workItems: StatePositionWorkItem[];
}

export interface StatePositionWorkListItemProps {
  failureDetail?: DashboardFailedWorkDetail;
  messages: StatePositionWorkListProps["messages"];
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  workItem: StatePositionWorkItem;
}

export interface StateNodeDetailCardProps {
  currentWorkItems: StatePositionWorkItem[];
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  place: DashboardPlaceRef;
  terminalHistoryWorkItems?: StatePositionWorkItem[];
  tokenCount: number;
  widgetId?: string;
}

export interface ExecutionDetailsSectionProps {
  activeTraceID?: string | null;
  details: SelectedWorkItemExecutionDetails;
  now: number;
  onSelectTraceID?: (traceID: string) => void;
  showInferenceAttempts?: boolean;
  traceTargetId: string;
}

export interface InferenceAttemptsSectionProps {
  attempts: DashboardInferenceAttempt[];
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedProviderSessionKey?: string | null;
}

export interface InferenceAttemptCardProps {
  attempt: DashboardInferenceAttempt;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedProviderSessionKey?: string | null;
}

export interface InferenceAttemptDetailProps {
  code?: boolean;
  label: string;
  value?: number | string;
}

export interface InferenceAttemptTextSectionProps {
  label: string;
  value: string;
}

export interface WorkItemDetailCardProps {
  activeTraceID?: string | null;
  dispatchAttempts: DashboardProviderSessionAttempt[];
  executionDetails: SelectedWorkItemExecutionDetails;
  locale?: string;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedNode?: DashboardWorkstationNode | null;
  selectedProviderSession?: LoadableProviderSessionRef | null;
  selectedProviderSessionKey?: string | null;
  selection: DashboardWorkItemSelection;
  selectedTrace?: DashboardTrace;
  workstationRequests: SelectedWorkRequestHistoryItem[];
  traceTargetId?: string;
  widgetId?: string;
}

export interface WorkstationDetailCardProps {
  activeExecutions: DashboardActiveExecution[];
  editableConfigurationState?: EditableWorkstationConfigurationState;
  headerAction?: ReactNode;
  locale?: string;
  now: number;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  providerSessions: DashboardProviderSessionAttempt[];
  saveState?: EditableWorkstationSaveState;
  selectedWorkID?: string | null;
  selectedProviderSession?: LoadableProviderSessionRef | null;
  selectedProviderSessionKey?: string | null;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedNode: DashboardWorkstationNode;
  workstationRequests?: DashboardWorkstationRequest[];
  widgetId?: string;
}

export interface EditableWorkstationValidationErrors {
  prompt?: string;
  workerName?: string;
}

export interface EditableWorkstationPromptDiagnostic {
  endOffset?: number;
  kind: string;
  message: string;
  path?: string;
  sourceText?: string;
  startOffset?: number;
}

export type EditableWorkstationPromptHelpState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { message: string; status: "empty" }
  | { contract: PromptTemplateContract; status: "ready" };

export type EditableWorkstationPromptValidationState =
  | { status: "idle" }
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | {
      diagnostics: EditableWorkstationPromptDiagnostic[];
      result: PromptTemplateValidationResult;
      status: "ready";
    };

export type EditableWorkstationOverwriteField = "prompt" | "worker";

export type EditableWorkstationWorkerOptionsState =
  | { status: "ready"; options: string[] }
  | { message: string; status: "empty" }
  | { message: string; status: "error" };

export type EditableWorkstationConfigurationState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { message: string; status: "empty" }
  | {
      draft: {
        prompt: string;
        workerName: string;
      };
      hasValidationErrors: boolean;
      initialValues: EditableWorkstationValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      onPromptChange: (value: string) => void;
      onWorkerChange: (value: string) => void;
      overwriteFieldNames: EditableWorkstationOverwriteField[];
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      promptDiagnostics: EditableWorkstationPromptDiagnostic[];
      promptHelpState: EditableWorkstationPromptHelpState;
      promptValidationState: EditableWorkstationPromptValidationState;
      status: "ready";
      validationErrors: EditableWorkstationValidationErrors;
      workerOptionsState: EditableWorkstationWorkerOptionsState;
    };

export type EditableWorkstationSaveState =
  | { status: "idle" }
  | { status: "confirming" }
  | { status: "submitting" }
  | { status: "success" }
  | { errorMessage: string; status: "error" };

export interface WorkstationActiveWorkListProps {
  executions: DashboardActiveExecution[];
  messages: WorkstationDetailMessages;
  now: number;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  selectedNode: DashboardWorkstationNode;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedWorkID?: string | null;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

export interface WorkstationSummaryProps {
  activeRunCount: number;
  historyCount: number;
  historyLabel: string;
  messages: WorkstationDetailMessages;
  selectedNode: DashboardWorkstationNode;
}

export interface WorkstationSummaryItemProps {
  label: string;
  value: string | number;
}

export interface ProviderSessionAttemptsProps {
  attempts: DashboardProviderSessionAttempt[];
  collapseActionLabel?: string;
  currentDispatchID?: string | null;
  emptyMessage: string;
  expandActionLabel?: string;
  historyItemCountLabel?: (count: number) => string;
  messages?: Pick<
    WorkstationDetailMessages,
    | "collapseAction"
    | "currentDispatchLabel"
    | "expandAction"
    | "historyRunCountLabel"
    | "openNamedWorkItemAction"
    | "openRequestDetailsAction"
    | "providerSessionLogAction"
    | "providerSessionLogUnavailable"
    | "providerSessionSelectedAction"
    | "providerSessionSelectAction"
    | "providerSessionSelectionUnavailable"
    | "requestDetailsUnavailable"
    | "requestSelectedAction"
    | "requestHistoryHeading"
    | "selectProviderSessionLabel"
    | "selectWorkItemLabel"
    | "selectWorkstationRequestLabel"
    | "workDetailsUnavailable"
    | "workSelectedAction"
    | "runHistoryHeading"
  >;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  renderHeading: (attempt: DashboardProviderSessionAttempt) => string;
  selectedProviderSessionKey?: string | null;
  selectedRequestDispatchID?: string | null;
  selectedWorkID?: string | null;
  workstationKind?: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
  title?: string;
}

export interface CollapsibleProviderSessionAttemptsProps
  extends ProviderSessionAttemptsProps {
  resetKey: string;
}

export interface ProviderSessionLogAccessProps {
  messages?: ProviderSessionAttemptsProps["messages"];
  session: DashboardProviderSession | undefined;
  startedAt?: string;
}

export type SelectedWorkRequestHistoryItem =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

export interface SelectedWorkDispatchHistorySectionProps {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  fallbackProviderSessions: DashboardProviderSessionAttempt[];
  locale?: string;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  requests: SelectedWorkRequestHistoryItem[];
  selectedProviderSessionKey?: string | null;
  selectedWorkID: string;
  traceTargetId: string;
  workstationKind?: string;
}

export interface WorkstationRequestDetailCardProps {
  onSelectWorkID?: (workID: string) => void;
  request: DashboardWorkstationRequest;
  selectedWorkID?: string | null;
  widgetId?: string;
}

export interface TerminalWorkSummaryCardProps {
  executionDetails?: SelectedWorkItemExecutionDetails;
  failureMessage?: string;
  failureReason?: string;
  label: string;
  now?: number;
  status: "completed" | "failed";
  widgetId?: string;
}

export interface MetadataSectionProps {
  emptyMessage: string;
  metadata: Record<string, string> | undefined;
  title: string;
}
