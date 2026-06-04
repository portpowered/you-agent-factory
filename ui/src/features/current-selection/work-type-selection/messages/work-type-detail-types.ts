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
  collapseAction: string;
  editableConfigurationCollapseActionLabel: string;
  editableConfigurationExpandActionLabel: string;
  editableConfigurationHeading: string;
  editableConfigurationSaveAction: string;
  expandAction: string;
  editableConfigurationSaveBusyAction: string;
  editableConfigurationSaveConfirmationCancelAction: string;
  editableConfigurationSaveConfirmationConfirmAction: string;
  editableConfigurationSaveConfirmationDescription: string;
  editableConfigurationSaveConfirmationTitle: string;
  editableConfigurationSaveErrorPrefix: string;
  editableConfigurationSaveFallbackError: string;
  editableConfigurationSaveStaleVersionDetail: string;
  editableConfigurationSaveSuccess: (workTypeName: string) => string;
  editableConfigurationValidationStatus: string;
  handlingBehaviorDefaultHelper: string;
  handlingBehaviorDefaultLabel: string;
  handlingBehaviorDefaultStatusLabel: string;
  localizeWorkStateType: (workStateType: WorkStateType) => string;
  selectWorkStateGraphNodeLabel: (stateName: string) => string;
  stateNameColumnLabel: string;
  statesEmpty: string;
  statesHeading: string;
  stateTypeColumnLabel: string;
  workTypeNameLabel: string;
}
