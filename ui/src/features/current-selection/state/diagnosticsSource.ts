
import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTraceDispatch,
  DashboardWorkDiagnostics,
} from "../../../api/dashboard";



export type ExecutionDetailSource =
  | "active-execution"
  | "provider-diagnostics"
  | "provider-session"
  | "trace"
  | "workstation-request"
  | "workstation";


export interface RuntimeDiagnosticsSource {
  diagnostics?: DashboardWorkDiagnostics;
  source: ExecutionDetailSource;
}

export function selectDiagnosticsSource(
  _workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): RuntimeDiagnosticsSource | undefined {
  if (activeExecution?.diagnostics) {
    return { diagnostics: activeExecution.diagnostics, source: "active-execution" };
  }
  if (matchingAttempt?.diagnostics) {
    return { diagnostics: matchingAttempt.diagnostics, source: "provider-diagnostics" };
  }
  if (matchingTraceDispatch?.diagnostics) {
    return { diagnostics: matchingTraceDispatch.diagnostics, source: "trace" };
  }
  return undefined;
}
