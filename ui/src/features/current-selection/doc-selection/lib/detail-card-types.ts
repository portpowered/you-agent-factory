import type { ReactNode } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type {
  EditableDocDraft,
  EditableDocValues,
} from "../../../current-factory-definition/lib/doc-editable-values";
import type { DetailCardSaveState } from "../../base/hooks/detail-card-save-types";
import type { EditableDocValidationErrors } from "./doc-editable-validation";

export interface DocDetailCardProps {
  editableConfigurationState?: EditableDocConfigurationState;
  headerAction?: ReactNode;
  locale?: string | null;
  saveState?: EditableDocSaveState;
  targetPath: string;
  widgetId?: string;
}

export type EditableDocSaveValidationErrors = {
  fileName?: string;
  inlineContent?: string;
} & Record<string, string>;

export type EditableDocOverwriteField = "fileName" | "inlineContent";

export type EditableDocSaveState =
  DetailCardSaveState<EditableDocSaveValidationErrors>;

export type EditableDocConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion: CurrentFactoryVersion;
      canSave: boolean;
      draft: EditableDocDraft;
      hasValidationErrors: boolean;
      initialValues: EditableDocValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      onFileNameChange: (value: string) => void;
      onInlineContentChange: (value: string) => void;
      onResetToLatest: () => void;
      originalTargetPath: string;
      overwriteFieldNames: EditableDocOverwriteField[];
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      pendingTargetPath: string;
      savedFactoryDefinition: CanonicalFactoryDefinition;
      status: "ready";
      validationErrors: EditableDocValidationErrors;
    };

export type { EditableDocValidationErrors } from "./doc-editable-validation";
