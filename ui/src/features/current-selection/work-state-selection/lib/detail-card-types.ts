import type { DashboardFailedWorkDetail, DashboardPlaceRef } from "../../../../api/dashboard/types";
import type { CurrentSelectionDetailMessages } from "../../base/messages/current-selection-detail";
import type { StatePositionWorkItem } from "../../base/state/selection-types";

export interface StatePositionWorkListProps {
  failedWorkDetailsByWorkID?: Record<string, DashboardFailedWorkDetail>;
  messages: Pick<
    CurrentSelectionDetailMessages,
    | "failureMessageLabel"
    | "failureReasonLabel"
    | "startedAtLabel"
    | "selectWorkItemLabel"
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
  onSelectWorkItem?: (workItem: StatePositionWorkItem) => void;
  place: DashboardPlaceRef;
  terminalHistoryWorkItems?: StatePositionWorkItem[];
  tokenCount: number;
  widgetId?: string;
}
