import type { EditableWorkTypeValidationMessages } from "../../../current-factory-definition/lib/work-type-editable-validation";

export interface WorkTypeDetailMessages
  extends EditableWorkTypeValidationMessages {
  configurationEmpty: string;
  topologyDeleteAction: (workTypeName: string) => string;
  topologyDeleteBlockedPrefix: string;
  topologyDeleteHeading: string;
}
