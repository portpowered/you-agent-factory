import type { ReactNode } from "react";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardPlaceRef,
  DashboardProviderSession,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import type { EditableWorkstationValues } from "../current-factory-definition/workstation-editable-values";
import type { WorkstationDetailMessages } from "./messages";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import type { DashboardWorkItemSelection } from "./types";

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
  onSelectWorkItem?: (workItem: DashboardWorkItemRef) => void;
  workItems: DashboardWorkItemRef[];
}

export interface StatePositionWorkListItemProps {
  failureDetail?: DashboardFailedWorkDetail;
  onSelectWorkItem?: (workItem: DashboardWorkItemRef) => void;
  workItem: DashboardWorkItemRef;
}

export interface StateNodeDetailCardProps {
  currentWorkItems: DashboardWorkItemRef[];
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  onSelectWorkItem?: (workItem: DashboardWorkItemRef) => void;
  place: DashboardPlaceRef;
  terminalHistoryWorkItems?: DashboardWorkItemRef[];
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
}

export interface InferenceAttemptCardProps {
  attempt: DashboardInferenceAttempt;
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
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  selectedNode?: DashboardWorkstationNode | null;
  selection: DashboardWorkItemSelection;
  selectedTrace?: DashboardTrace;
  workstationRequests: SelectedWorkRequestHistoryItem[];
  traceTargetId?: string;
  widgetId?: string;
}

export interface WorkstationDetailCardProps {
  activeExecutions: DashboardActiveExecution[];
  editableConfigurationState?: EditableWorkstationConfigurationState;
  locale?: string;
  now: number;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  providerSessions: DashboardProviderSessionAttempt[];
  selectedWorkID?: string | null;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedNode: DashboardWorkstationNode;
  workstationRequests?: DashboardWorkstationRequest[];
  widgetId?: string;
}

export interface EditableWorkstationValidationErrors {
  model?: string;
  prompt?: string;
  promptFile?: string;
}

export type EditableWorkstationConfigurationState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { message: string; status: "empty" }
  | {
      draft: {
        model: string;
        prompt: string;
        promptFile: string;
      };
      hasValidationErrors: boolean;
      initialValues: EditableWorkstationValues;
      isDirty: boolean;
      onModelChange: (value: string) => void;
      onPromptChange: (value: string) => void;
      onPromptFileChange: (value: string) => void;
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      status: "ready";
      validationErrors: EditableWorkstationValidationErrors;
    };

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
    | "currentDispatchLabel"
    | "openNamedWorkItemAction"
    | "openRequestDetailsAction"
    | "providerSessionLogAction"
    | "providerSessionLogUnavailable"
    | "requestDetailsUnavailable"
    | "requestSelectedAction"
    | "selectWorkItemLabel"
    | "selectWorkstationRequestLabel"
    | "workDetailsUnavailable"
    | "workSelectedAction"
  >;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  renderHeading: (attempt: DashboardProviderSessionAttempt) => string;
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
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  requests: SelectedWorkRequestHistoryItem[];
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

export interface RequestCountSectionProps {
  request: DashboardWorkstationRequest;
}

export interface MetadataSectionProps {
  emptyMessage: string;
  metadata: Record<string, string> | undefined;
  title: string;
}
