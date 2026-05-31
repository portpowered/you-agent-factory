import type { DashboardFailedWorkDetail, DashboardPlaceRef } from "../../../../api/dashboard/types";
import type {
  CanonicalFactoryDefinition,
  CurrentFactoryVersion,
} from "../../../../api/current-factory-definition";
import type {
  EditableWorkStateDraft,
  EditableWorkStateValues,
} from "../../../current-factory-definition/lib/work-state-editable-values";
import type { CurrentSelectionDetailMessages } from "../../base/messages/current-selection-detail";
import type { StatePositionWorkItem } from "../../base/state/selection-types";
import type { DetailCardSaveState } from "../../base/hooks/detail-card-save-types";
import type { EditableWorkStateValidationErrors } from "./work-state-editable-validation";

export type EditableWorkStateSaveValidationErrors = {
  contract?: string;
  name?: string;
} & Record<string, string>;

export type EditableWorkStateSaveState =
  DetailCardSaveState<EditableWorkStateSaveValidationErrors>;

export type EditableWorkStateConfigurationState =
  | { status: "loading" }
  | { status: "error"; errorMessage: string }
  | { status: "empty"; message?: string }
  | {
      baseVersion: CurrentFactoryVersion;
      canSave: boolean;
      draft: EditableWorkStateDraft;
      hasValidationErrors: boolean;
      initialValues: EditableWorkStateValues;
      isDirty: boolean;
      markChangesSaved: () => void;
      onNameChange: (value: string) => void;
      onResetToLatest: () => void;
      originalStateName: string;
      pendingFactoryDefinition: CanonicalFactoryDefinition | null;
      status: "ready";
      validationErrors: EditableWorkStateValidationErrors;
      workTypeName: string;
    };

export type { EditableWorkStateValidationErrors } from "./work-state-editable-validation";

export interface StatePositionWorkListProps {
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  messages: Pick<
    CurrentSelectionDetailMessages,
    | "failureMessageLabel"
    | "failureReasonLabel"
    | "startedAtLabel"
    | "selectWorkItemLabel"
    | "timestampUnavailable"
  >;
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  workItems: StatePositionWorkItem[];
}

export interface StatePositionWorkListItemProps {
  failureDetail?: DashboardFailedWorkDetail;
  messages: StatePositionWorkListProps["messages"];
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  workItem: StatePositionWorkItem;
}

export interface StateNodeDetailCardProps {
  currentWorkItems: StatePositionWorkItem[];
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  locale?: string | null;
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  place: DashboardPlaceRef;
  terminalHistoryWorkItems?: StatePositionWorkItem[];
  tokenCount: number;
  widgetId?: string;
}
