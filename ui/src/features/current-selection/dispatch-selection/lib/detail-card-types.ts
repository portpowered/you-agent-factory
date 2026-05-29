import type {
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardWorkstationRequest,
} from "../../../../api/dashboard/types";
import type { LoadableProviderSessionRef } from "../../../provider-session-detail/lib/provider-session-ref";

export type SelectedWorkRequestHistoryItem =
  | DashboardRuntimeWorkstationRequest
  | DashboardWorkstationRequest;

export interface SelectedWorkDispatchHistorySectionProps {
  activeTraceID?: string | null;
  currentDispatchID?: string | null;
  fallbackProviderSessions: DashboardProviderSessionAttempt[];
  locale?: string;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  onSelectTraceID?: (traceID: string) => void;
  onSelectWorkID?: (workID: string) => void;
  requests: SelectedWorkRequestHistoryItem[];
  selectedProviderSessionKey?: string | null;
  selectedWorkID: string;
  traceTargetId: string;
  workstationKind?: string;
}

export interface WorkstationRequestDetailCardProps {
  onSelectWorkID?: (workID: string) => void;
  request: DashboardWorkstationRequest;
  selectedWorkID?: string | null;
  widgetId?: string;
}
