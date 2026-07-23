import type { ReactNode } from "react";
import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../../api/current-factory-definition";
import type {
  PromptTemplateContract,
  PromptTemplateValidationResult,
} from "../../../../../api/current-factory-prompt-template";
import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardWorkstationNode,
  DashboardWorkstationRequest,
} from "../../../../../api/dashboard/types";
import type { components } from "../../../../../api/generated/openapi";
import type { EditableModelInvokeBindingDraft } from "../../../../current-factory-definition/lib/workstation/workstation-model-invoke";
import type { EditableWorkstationBehavior } from "../../../../current-factory-definition/lib/workstation-behavior";
import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../../current-factory-definition/lib/workstation-editable-values";
import type { LoadableProviderSessionRef } from "../../../../provider-session-detail/lib/provider-session-ref";
import type { DetailCardSaveState } from "../../../base/hooks/detail-card-save-types";
import type { ApiRunnerID } from "../../messages/runner-openapi-enums";
import type { WorkstationDetailMessages } from "../../messages/workstation-detail-types";

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

export type EditableWorkstationValidationErrors = {
  behavior?: string;
  cronExpiryWindow?: string;
  cronJitter?: string;
  cronSchedule?: string;
  cronTriggerAtStart?: string;
  name?: string;
  operation?: string;
  operationBindings?: string;
  prompt?: string;
  runnerName?: string;
  workerName?: string;
} & Record<string, string | undefined>;

export type EditableWorkstationSaveValidationErrors = {
  behavior?: string;
  cronExpiryWindow?: string;
  cronJitter?: string;
  cronSchedule?: string;
  cronTriggerAtStart?: string;
  name?: string;
  operation?: string;
  operationBindings?: string;
  prompt?: string;
  runnerName?: string;
  workerName?: string;
} & Record<string, string>;

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
  | "cronExpiryWindow"
  | "cronJitter"
  | "cronSchedule"
  | "cronTriggerAtStart"
  | "name"
  | "operation"
  | "operationBindings"
  | "prompt"
  | "runner"
  | "worker"
  | "workstationType";

export type EditableWorkstationWorkerOptionsState =
  | { status: "ready"; options: string[] }
  | { message: string; status: "empty" }
  | { message: string; status: "error" };

export type EditableWorkstationOperationOptionsState =
  | {
      operations: components["schemas"]["ModelOperation"][];
      options: string[];
      status: "ready";
    }
  | { message: string; status: "empty" }
  | { message: string; status: "error" };

export type EditableWorkstationWorkstationOptionsState =
  | { status: "ready"; options: string[] }
  | { message: string; status: "empty" }
  | { message: string; status: "error" };

export type EditableWorkstationConfigurationState =
  | { status: "loading" }
  | { errorMessage: string; status: "error" }
  | { message: string; status: "empty" }
  | {
      draft: EditableWorkstationDraft;
      hasValidationErrors: boolean;
      initialValues: EditableWorkstationValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      baseVersion: CurrentFactoryVersion;
      onBehaviorChange: (value: EditableWorkstationBehavior) => void;
      onCronExpiryWindowChange: (value: string) => void;
      onCronJitterChange: (value: string) => void;
      onCronScheduleChange: (value: string) => void;
      onCronTriggerAtStartChange: (value: boolean) => void;
      onNameChange: (value: string) => void;
      onOperationBindingsChange: (
        bindings: EditableModelInvokeBindingDraft[],
      ) => void;
      onOperationChange: (value: string) => void;
      onPromptChange: (value: string) => void;
      onResetToLatest: () => void;
      onGuardsChange: (guards: EditableWorkstationDraft["guards"]) => void;
      onInputsChange: (inputs: EditableWorkstationDraft["inputs"]) => void;
      onRunnerChange: (value: ApiRunnerID | null) => void;
      onWorkstationTypeChange: (
        value: EditableWorkstationValues["workstationType"],
      ) => void;
      onWorkerChange: (value: string) => void;
      operationOptionsState: EditableWorkstationOperationOptionsState;
      workstationOptionsState: EditableWorkstationWorkstationOptionsState;
      overwriteFieldNames: EditableWorkstationOverwriteField[];
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      savedFactoryDefinition: CanonicalFactoryDefinition;
      promptDiagnostics: EditableWorkstationPromptDiagnostic[];
      promptHelpState: EditableWorkstationPromptHelpState;
      promptValidationState: EditableWorkstationPromptValidationState;
      status: "ready";
      validationErrors: EditableWorkstationValidationErrors;
      workerOptionsState: EditableWorkstationWorkerOptionsState;
    };

export type EditableWorkstationSaveState =
  DetailCardSaveState<EditableWorkstationSaveValidationErrors>;

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

export interface WorkstationRequestHistorySectionProps {
  messages: WorkstationDetailMessages;
  now: number;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  requests: DashboardWorkstationRequest[];
  resetKey: string;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedWorkID?: string | null;
}

export interface WorkstationHistorySectionProps {
  collapseActionLabel: string;
  expandActionLabel: string;
  messages: WorkstationDetailMessages;
  now: number;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  providerSessions: DashboardProviderSessionAttempt[];
  selectedNodeID: string;
  selectedProviderSessionKey?: string | null;
  selectedRequest?: DashboardWorkstationRequest | null;
  selectedWorkID?: string | null;
  workstationKind?: DashboardWorkstationNode["workstation_kind"];
  workstationRequests: DashboardWorkstationRequest[];
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>;
}

export interface WorkstationSummaryProps {
  activeRunCount: number;
  editableConfigurationState?: EditableWorkstationConfigurationState;
  historyCount: number;
  historyLabel: string;
  locale?: string;
  messages: WorkstationDetailMessages;
  selectedNode: DashboardWorkstationNode;
}

export interface WorkstationSummaryItemProps {
  iconClassName?: string;
  iconKind?: string;
  iconLabel?: string;
  label: string;
  value: string | number;
}
