import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { FactoryWorker } from "../../../../api/events/types";
import type {
  EditableWorkerDraft,
  EditableWorkerValues,
} from "../../../current-factory-definition/lib/worker-editable-values";
import type { EditableWorkerValidationErrors } from "./worker-editable-validation";

export interface WorkerDetailCardProps {
  editableConfigurationState?: EditableWorkerConfigurationState;
  locale?: string | null;
  onSaveWorker?: () => void;
  saveState?: EditableWorkerSaveState;
  widgetId?: string;
  workerName: string;
}

export type EditableWorkerSaveValidationErrors = Partial<
  Record<
    | "args"
    | "body"
    | "command"
    | "executorProvider"
    | "model"
    | "modelLocality"
    | "modelProvider"
    | "provider"
    | "type",
    string
  >
>;

export type EditableWorkerSaveState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "success" }
  | { message: string; status: "warning" }
  | {
      errorMessage: string;
      fieldErrors?: EditableWorkerSaveValidationErrors;
      status: "error";
    };

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
      onModelLocalityChange: (value: EditableWorkerDraft["modelLocality"]) => void;
      onModelProviderChange: (
        value: EditableWorkerDraft["modelProvider"],
      ) => void;
      onProviderChange: (value: EditableWorkerDraft["provider"]) => void;
      markChangesSaved: () => void;
      onResetToLatest: () => void;
      onTypeChange: (value: EditableWorkerDraft["type"]) => void;
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
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
