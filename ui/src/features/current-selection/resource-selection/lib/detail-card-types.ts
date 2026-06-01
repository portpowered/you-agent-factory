import type { ReactNode } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { DetailCardSaveState } from "../../base/hooks/detail-card-save-types";
import type { FactoryResource } from "../../../../api/events/types";
import type {
  EditableResourceDraft,
  EditableResourceValues,
} from "../../../current-factory-definition/lib/resource-editable-values";
import type { EditableResourceValidationErrors } from "./resource-editable-validation";

export interface ResourceDetailCardProps {
  editableConfigurationState?: EditableResourceConfigurationState;
  headerAction?: ReactNode;
  locale?: string | null;
  resourceName: string;
  saveState?: EditableResourceSaveState;
  tokenCount?: number | null;
  widgetId?: string;
}

export type EditableResourceSaveValidationErrors = {
  backend?: string;
  capacity?: string;
  loadPolicy?: string;
  model?: string;
  name?: string;
  provider?: string;
  type?: string;
} & Record<string, string>;

export type EditableResourceSaveState =
  DetailCardSaveState<EditableResourceSaveValidationErrors>;

export type ResourceDetailState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty" }
  | {
      status: "ready";
      resource: FactoryResource;
      workerNames: string[];
      workstationNames: string[];
    };

export type EditableResourceOverwriteField =
  | "backend"
  | "capacity"
  | "loadPolicy"
  | "model"
  | "name"
  | "provider"
  | "type";

export type EditableResourceConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion: CurrentFactoryVersion;
      canSave: boolean;
      draft: EditableResourceDraft;
      hasValidationErrors: boolean;
      initialValues: EditableResourceValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      onBackendChange: (value: string) => void;
      onCapacityChange: (value: string) => void;
      onLoadPolicyChange: (value: string) => void;
      onModelChange: (value: string) => void;
      onNameChange: (value: string) => void;
      onProviderChange: (value: string) => void;
      onResetToLatest: () => void;
      onTypeChange: (value: EditableResourceDraft["type"]) => void;
      overwriteFieldNames: EditableResourceOverwriteField[];
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      savedFactoryDefinition: CanonicalFactoryDefinition;
      status: "ready";
      validationErrors: EditableResourceValidationErrors;
    };

export type { EditableResourceValidationErrors } from "./resource-editable-validation";
