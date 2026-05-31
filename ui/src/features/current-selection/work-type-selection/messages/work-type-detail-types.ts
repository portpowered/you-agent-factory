import type { EditableWorkTypeValidationMessages } from "../../../current-factory-definition/lib/work-type-editable-validation";
import type { EditableWorkTypeValues } from "../../../current-factory-definition/lib/work-type-editable-values";

type WorkStateType = NonNullable<
  EditableWorkTypeValues["states"]
>[number]["type"];

export interface WorkTypeDetailMessages
  extends EditableWorkTypeValidationMessages {
  configurationEmpty: string;
  configurationErrorPrefix: string;
  configurationLoading: string;
  localizeWorkStateType: (workStateType: WorkStateType) => string;
  stateNameColumnLabel: string;
  statesEmpty: string;
  statesHeading: string;
  stateTypeColumnLabel: string;
  topologyDeleteAction: (workTypeName: string) => string;
  topologyDeleteBlockedPrefix: string;
  topologyDeleteHeading: string;
  workTypeNameLabel: string;
}
