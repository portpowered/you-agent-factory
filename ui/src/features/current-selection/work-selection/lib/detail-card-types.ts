import type {
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardTrace,
  DashboardWorkstationNode,
} from "../../../../api/dashboard/types";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";
import type { SelectedWorkRequestHistoryItem } from "../../base/components/detail-card-types";
import type { DashboardWorkItemSelection } from "../../base/state/selection-types";
import type { SelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";
import type { SelectedWorkItemExecutionDetails } from "../state/executionDetails";

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

export type {
  InferenceAttemptDetailProps,
  InferenceAttemptTextSectionProps,
} from "../../base/components/detail-card-types";

export interface WorkItemDetailCardProps {
  activeTraceID?: string | null;
  dispatchAttempts: DashboardProviderSessionAttempt[];
  executionDetails: SelectedWorkItemExecutionDetails;
  locale?: string;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  relationshipGraph?: SelectedWorkRelationshipGraph;
  selectedNode?: DashboardWorkstationNode | null;
  selectedProviderSessionKey?: string | null;
  selection: DashboardWorkItemSelection;
  selectedTrace?: DashboardTrace;
  workstationRequests: SelectedWorkRequestHistoryItem[];
  traceTargetId?: string;
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
