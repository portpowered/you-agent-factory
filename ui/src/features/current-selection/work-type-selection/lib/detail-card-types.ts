import type { ReactNode } from "react";

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { DetailCardSaveState } from "../../base/hooks/detail-card-save-types";
import type { EditableWorkTypeValidationErrors } from "../../../current-factory-definition/lib/work-type-editable-validation";
import type {
  EditableWorkTypeDraft,
  EditableWorkTypeValues,
} from "../../../current-factory-definition/lib/work-type-editable-values";

export type EditableWorkTypeSaveValidationErrors = {
  handlingBehavior?: string;
  name?: string;
} & Record<string, string>;

export type EditableWorkTypeSaveState =
  DetailCardSaveState<EditableWorkTypeSaveValidationErrors>;

export type EditableWorkTypeConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion: CurrentFactoryVersion;
      canSave: boolean;
      draft: EditableWorkTypeDraft;
      hasValidationErrors: boolean;
      initialValues: EditableWorkTypeValues;
      isDirty: boolean;
      onHandlingBehaviorChange: (
        value: EditableWorkTypeDraft["handlingBehavior"],
      ) => void;
      onNameChange: (value: string) => void;
      markChangesSaved: () => void;
      onResetToLatest: () => void;
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      savedFactoryDefinition: CanonicalFactoryDefinition;
      status: "ready";
      validationErrors: EditableWorkTypeValidationErrors;
    };

export type { EditableWorkTypeValidationErrors };

export interface WorkTypeDetailCardProps {
  editableConfigurationState?: EditableWorkTypeConfigurationState;
  headerAction?: ReactNode;
  locale?: string | null;
  onSelectWorkStateGraphNode?: (graphNodeId: string) => void;
  saveState?: EditableWorkTypeSaveState;
  widgetId?: string;
  workTypeName: string;
}
