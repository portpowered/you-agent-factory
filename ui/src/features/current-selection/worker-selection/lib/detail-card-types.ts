import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { FactoryWorker } from "../../../../api/events/types";
import type {
  EditableWorkerDraft,
  EditableWorkerValues,
} from "../../../current-factory-definition/lib/worker-editable-values";

export interface WorkerDetailCardProps {
  editableConfigurationState?: EditableWorkerConfigurationState;
  locale?: string | null;
  widgetId?: string;
  workerName: string;
}

export type EditableWorkerConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion?: CurrentFactoryVersion;
      draft: EditableWorkerDraft;
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
      onResetToLatest: () => void;
      onTypeChange: (value: EditableWorkerDraft["type"]) => void;
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      status: "ready";
    };

export type WorkerDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      worker: FactoryWorker;
      workstationNames: string[];
    };
