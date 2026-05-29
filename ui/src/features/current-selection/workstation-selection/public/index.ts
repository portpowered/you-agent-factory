export {
  WorkstationDetailCard,
} from "../components/workstation-detail-card";

export {
  EditableWorkstationSaveDialog,
  EditableWorkstationSaveHeaderAction,
} from "../components/workstation-save-controls";

export {
  CollapsibleProviderSessionAttempts,
  ProviderSessionAttempts,
} from "../components/provider-session-attempts";

export { useEditableWorkstationConfigurationState } from "../hooks/use-editable-workstation-configuration-state";
export { useSaveEditableWorkstationConfiguration } from "../hooks/use-save-editable-workstation-configuration";

export {
  getWorkstationDetailMessages,
  workstationDetailMessagesByLocale,
} from "../messages/workstation-detail";
export type { WorkstationDetailMessages } from "../messages/workstation-detail-types";

export type {
  EditableWorkstationConfigurationState,
  EditableWorkstationOverwriteField,
  EditableWorkstationSaveState,
  WorkstationDetailCardProps,
} from "../lib/detail-card-types";
