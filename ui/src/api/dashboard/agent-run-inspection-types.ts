export interface DashboardAgentRunInspection {
  execution_behavior?: string;
  executionBehavior?: string;
  failure_class?: string;
  failureClass?: string;
  recovery_action?: string;
  recoveryAction?: string;
  tool_policy?: string;
  toolPolicy?: string;
  tool_call_count?: number;
  toolCallCount?: number;
  tool_diagnostics?: DashboardAgentRunToolDiagnostic[];
  toolDiagnostics?: DashboardAgentRunToolDiagnostic[];
  transcript?: DashboardAgentRunTranscriptEntry[];
}

export interface DashboardAgentRunToolDiagnostic {
  tool_name?: string;
  toolName?: string;
  phase?: string;
  detail?: string;
}

export interface DashboardAgentRunTranscriptEntry {
  role?: string;
  summary?: string;
}

export function toDashboardAgentRunInspection(
  inspection: DashboardAgentRunInspection | undefined,
): DashboardAgentRunInspection | undefined {
  if (!inspection) {
    return undefined;
  }
  const toolDiagnostics = (
    inspection.tool_diagnostics ?? inspection.toolDiagnostics
  )?.map((entry) => ({
    tool_name: entry.tool_name ?? entry.toolName,
    phase: entry.phase,
    detail: entry.detail,
  }));
  return {
    execution_behavior:
      inspection.execution_behavior ?? inspection.executionBehavior,
    failure_class: inspection.failure_class ?? inspection.failureClass,
    recovery_action: inspection.recovery_action ?? inspection.recoveryAction,
    tool_policy: inspection.tool_policy ?? inspection.toolPolicy,
    tool_call_count: inspection.tool_call_count ?? inspection.toolCallCount,
    tool_diagnostics: toolDiagnostics,
    transcript: inspection.transcript,
  };
}
