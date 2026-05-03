
import type {
  DashboardActiveExecution,
  DashboardProviderDiagnostic,
  DashboardProviderSessionAttempt,
  DashboardRenderedPromptDiagnostic,
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
  workstationRequest: DashboardRuntimeWorkstationRequest | undefined,
  activeExecution: DashboardActiveExecution | undefined,
  matchingAttempt: DashboardProviderSessionAttempt | undefined,
  matchingTraceDispatch: DashboardTraceDispatch | undefined,
): RuntimeDiagnosticsSource | undefined {
  if (workstationRequest?.response?.diagnostics) {
    return {
      diagnostics: normalizeRuntimeDiagnostics(workstationRequest.response.diagnostics),
      source: "workstation-request",
    };
  }
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



function normalizeRuntimeDiagnostics(
  diagnostics: NonNullable<
    NonNullable<DashboardRuntimeWorkstationRequest["response"]>["diagnostics"]
  >,
): DashboardWorkDiagnostics {
  const runtimeDiagnostics = diagnostics as {
    provider?: {
      model?: string;
      provider?: string;
      requestMetadata?: Record<string, string>;
      responseMetadata?: Record<string, string>;
      request_metadata?: Record<string, string>;
      response_metadata?: Record<string, string>;
    };
    renderedPrompt?: {
      systemPromptHash?: string;
      userMessageHash?: string;
      variables?: Record<string, string>;
    };
    rendered_prompt?: {
      system_prompt_hash?: string;
      user_message_hash?: string;
      variables?: Record<string, string>;
    };
  };

  const provider: DashboardProviderDiagnostic | undefined = runtimeDiagnostics.provider
    ? {
        model: runtimeDiagnostics.provider.model,
        provider: runtimeDiagnostics.provider.provider,
        request_metadata:
          runtimeDiagnostics.provider.requestMetadata ??
          runtimeDiagnostics.provider.request_metadata,
        response_metadata:
          runtimeDiagnostics.provider.responseMetadata ??
          runtimeDiagnostics.provider.response_metadata,
      }
    : undefined;
  const renderedPrompt: DashboardRenderedPromptDiagnostic | undefined =
    runtimeDiagnostics.renderedPrompt || runtimeDiagnostics.rendered_prompt
      ? {
          system_prompt_hash:
            runtimeDiagnostics.renderedPrompt?.systemPromptHash ??
            runtimeDiagnostics.rendered_prompt?.system_prompt_hash,
          user_message_hash:
            runtimeDiagnostics.renderedPrompt?.userMessageHash ??
            runtimeDiagnostics.rendered_prompt?.user_message_hash,
          variables:
            runtimeDiagnostics.renderedPrompt?.variables ??
            runtimeDiagnostics.rendered_prompt?.variables,
        }
      : undefined;

  return {
    provider,
    rendered_prompt: renderedPrompt,
  };
}

