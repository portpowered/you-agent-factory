import type { ReactNode } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { FactoryWorker } from "../../../../api/events/types";
import type {
  EditableWorkerDraft,
  EditableWorkerValues,
} from "../../../current-factory-definition/lib/worker-editable-values";
import type { DetailCardSaveState } from "../../base/hooks/detail-card-save-types";
import type { EditableWorkerValidationErrors } from "./worker-editable-validation";

export interface WorkerDetailCardProps {
  editableConfigurationState?: EditableWorkerConfigurationState;
  headerAction?: ReactNode;
  locale?: string | null;
  onSaveConfiguration?: () => void;
  saveState?: EditableWorkerSaveState;
  widgetId?: string;
  workerName: string;
}

export type EditableWorkerSaveValidationErrors = {
  args?: string;
  body?: string;
  command?: string;
  executorProvider?: string;
  model?: string;
  modelLocality?: string;
  modelProvider?: string;
  name?: string;
  provider?: string;
  skipPermissions?: string;
  stopToken?: string;
  timeout?: string;
  type?: string;
} & Record<string, string>;

export type EditableWorkerOverwriteField =
  | "args"
  | "body"
  | "command"
  | "executorProvider"
  | "model"
  | "modelLocality"
  | "modelProvider"
  | "name"
  | "provider"
  | "skipPermissions"
  | "stopToken"
  | "timeout"
  | "type";

export type EditableWorkerSaveState =
  DetailCardSaveState<EditableWorkerSaveValidationErrors>;

export type EditableWorkerConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion: CurrentFactoryVersion;
      canSave: boolean;
      draft: EditableWorkerDraft;
      hasValidationErrors: boolean;
      initialValues: EditableWorkerValues;
      isDirty: boolean;
      onArgsTextChange: (value: string) => void;
      onBodyChange: (value: string) => void;
      onCommandChange: (value: string) => void;
      onExecutorProviderChange: (
        value: EditableWorkerDraft["executorProvider"],
      ) => void;
      onModelChange: (value: string) => void;
      onModelLocalityChange: (
        value: EditableWorkerDraft["modelLocality"],
      ) => void;
      onModelProviderChange: (
        value: EditableWorkerDraft["modelProvider"],
      ) => void;
      onNameChange: (value: string) => void;
      onProviderChange: (value: EditableWorkerDraft["provider"]) => void;
      onSkipPermissionsChange: (value: boolean) => void;
      onStopTokenChange: (value: string) => void;
      onTimeoutAmountChange: (value: string) => void;
      onTimeoutUnitChange: (
        value: EditableWorkerDraft["timeoutUnit"],
      ) => void;
      markChangesSaved: () => void;
      onResetToLatest: () => void;
      onTypeChange: (value: EditableWorkerDraft["type"]) => void;
      overwriteFieldNames: EditableWorkerOverwriteField[];
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      savedFactoryDefinition: CanonicalFactoryDefinition;
      status: "ready";
      validationErrors: EditableWorkerValidationErrors;
    };

export type { EditableWorkerValidationErrors } from "./worker-editable-validation";

export type WorkerDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      worker: FactoryWorker;
      workstationNames: string[];
    };
