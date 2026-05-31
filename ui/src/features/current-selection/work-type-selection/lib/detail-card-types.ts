import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type { EditableWorkTypeValidationErrors } from "../../../current-factory-definition/lib/work-type-editable-validation";
import type {
  EditableWorkTypeDraft,
  EditableWorkTypeValues,
} from "../../../current-factory-definition/lib/work-type-editable-values";

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
      status: "ready";
      validationErrors: EditableWorkTypeValidationErrors;
    };

export type { EditableWorkTypeValidationErrors };
