import type { ReactNode } from "react";
import type {
  DashboardFailedWorkDetail,
  DashboardPlaceRef,
  DashboardProviderSession,
  DashboardProviderSessionAttempt,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";
import type { CurrentSelectionDetailMessages } from "../messages/current-selection-detail";
import type { WorkstationDetailMessages } from "../../messages/workstation-detail-types";
import type { StatePositionWorkItem } from "../state/selection-types";

export interface SelectionDetailLayoutProps {
  children: ReactNode;
  headerAction?: ReactNode;
  widgetId?: string;
}

export interface NoSelectionDetailCardProps {
  widgetId?: string;
}

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

export interface InferenceAttemptDetailProps {
  code?: boolean;
  label: string;
  rawValue?: string;
  value?: number | string;
}

export interface InferenceAttemptTextSectionProps {
  label: string;
  value: string;
}

export interface ProviderSessionAttemptsProps {
  attempts: DashboardProviderSessionAttempt[];
  collapseActionLabel?: string;
  currentDispatchID?: string | null;
  emptyMessage: string;
  expandActionLabel?: string;
  historyItemCountLabel?: (count: number) => string;
  messages?: Pick<
    WorkstationDetailMessages,
    | "collapseAction"
    | "currentDispatchLabel"
    | "expandAction"
    | "historyRunCountLabel"
    | "localizeProviderSessionKind"
    | "openNamedWorkItemAction"
    | "openRequestDetailsAction"
    | "providerSessionLogAction"
    | "providerSessionLogUnavailable"
    | "providerSessionSelectedAction"
    | "providerSessionSelectAction"
    | "providerSessionSelectionUnavailable"
    | "requestDetailsUnavailable"
    | "requestSelectedAction"
    | "requestHistoryHeading"
    | "selectProviderSessionLabel"
    | "selectWorkItemLabel"
    | "selectWorkstationRequestLabel"
    | "workDetailsUnavailable"
    | "workSelectedAction"
    | "runHistoryHeading"
    | "unavailableValue"
  >;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectWorkID?: (workID: string) => void;
  onSelectWorkstationRequest?: (request: DashboardWorkstationRequest) => void;
  renderHeading: (attempt: DashboardProviderSessionAttempt) => string;
  selectedProviderSessionKey?: string | null;
  selectedRequestDispatchID?: string | null;
  selectedWorkID?: string | null;
  workstationKind?: string;
  workstationRequestsByDispatchID?: Record<string, DashboardWorkstationRequest>;
  title?: string;
}

export interface CollapsibleProviderSessionAttemptsProps
  extends ProviderSessionAttemptsProps {
  resetKey: string;
}

export interface ProviderSessionLogAccessProps {
  messages?: ProviderSessionAttemptsProps["messages"];
  session: DashboardProviderSession | undefined;
  startedAt?: string;
}

export type {
  SelectedWorkDispatchHistorySectionProps,
  SelectedWorkRequestHistoryItem,
  WorkstationRequestDetailCardProps,
} from "../../dispatch-selection/lib/detail-card-types";

export interface MetadataSectionProps {
  emptyMessage: string;
  metadata: Record<string, string> | undefined;
  title: string;
}
