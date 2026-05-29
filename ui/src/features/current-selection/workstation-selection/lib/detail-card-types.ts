import type { ReactNode } from "react";
import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type {
  PromptTemplateContract,
  PromptTemplateValidationResult,
} from "../../../../api/current-factory-prompt-template";
import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import type { EditableWorkstationBehavior } from "../../../current-factory-definition/lib/workstation-behavior";
import type { EditableWorkstationValues } from "../../../current-factory-definition/lib/workstation-editable-values";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";
import type { RunnerID } from "../editing/runner-metadata";
import type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

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
  selectedProviderSessionKey?: string | null;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedNode: DashboardWorkstationNode;
  workstationRequests?: DashboardWorkstationRequest[];
  widgetId?: string;
}

export interface EditableWorkstationValidationErrors {
  behavior?: string;
  prompt?: string;
  runnerName?: string;
  workerName?: string;
}

export interface EditableWorkstationSaveValidationErrors {
  behavior?: string;
  prompt?: string;
  runnerName?: string;
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

export type EditableWorkstationOverwriteField =
  | "behavior"
  | "prompt"
  | "runner"
  | "worker";

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
        behavior: EditableWorkstationBehavior;
        prompt: string;
        runnerName: RunnerID | null;
        workerName: string;
      };
      hasValidationErrors: boolean;
      initialValues: EditableWorkstationValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      baseVersion: CurrentFactoryVersion;
      onBehaviorChange: (value: EditableWorkstationBehavior) => void;
      onPromptChange: (value: string) => void;
      onResetToLatest: () => void;
      onRunnerChange: (value: RunnerID | null) => void;
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
  | { message: string; status: "warning" }
  | {
      errorMessage: string;
      fieldErrors?: EditableWorkstationSaveValidationErrors;
      status: "error";
    };

export interface WorkstationActiveWorkListProps {
  executions: DashboardActiveExecution[];
  messages: WorkstationDetailMessages;
  now: number;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  selectedNodeID: string;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedWorkID?: string | null;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
}

export interface WorkstationSummaryProps {
  activeRunCount: number;
  editableConfigurationState?: EditableWorkstationConfigurationState;
  historyCount: number;
  historyLabel: string;
  messages: WorkstationDetailMessages;
  selectedNode: DashboardWorkstationNode;
}

export interface WorkstationSummaryItemProps {
  label: string;
  value: string | number;
}
