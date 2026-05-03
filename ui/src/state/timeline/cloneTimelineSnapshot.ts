import type {
  DashboardFailedWorkDetail,
  DashboardInferenceAttempt,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardTraceMutation,
  DashboardTraceToken,
  DashboardWorkDiagnostics,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard";
import type { FactoryRelation } from "../../api/events";
import type { TimelineWorkRequestPayload } from "./types";

export function cloneRelationsByWorkID(
  relationsByWorkID: Record<string, FactoryRelation[]>,
): Record<string, FactoryRelation[]> {
  return Object.fromEntries(
    Object.entries(relationsByWorkID).map(([workID, relations]) => [
      workID,
      relations.map((relation) => ({ ...relation })),
    ]),
  );
}

export function uniqueSortedWorkRefs(
  workItems: DashboardWorkItemRef[],
): DashboardWorkItemRef[] {
  const itemsByID = new Map<string, DashboardWorkItemRef>();

  for (const item of workItems) {
    if (!item.work_id) {
      continue;
    }

    itemsByID.set(item.work_id, item);
  }

  return [...itemsByID.values()].sort((left, right) => left.work_id.localeCompare(right.work_id));
}

export function cloneWorkItemRef(item: DashboardWorkItemRef): DashboardWorkItemRef {
  return {
    ...item,
    previous_chaining_trace_ids: item.previous_chaining_trace_ids
      ? [...item.previous_chaining_trace_ids]
      : undefined,
  };
}

function cloneTraceToken(token: DashboardTraceToken): DashboardTraceToken {
  return {
    ...token,
    tags: token.tags ? { ...token.tags } : undefined,
  };
}

function cloneTraceMutation(mutation: DashboardTraceMutation): DashboardTraceMutation {
  return {
    ...mutation,
    resulting_token: mutation.resulting_token
      ? cloneTraceToken(mutation.resulting_token)
      : undefined,
  };
}

function cloneTraceDispatch(dispatch: DashboardTraceDispatch): DashboardTraceDispatch {
  return {
    ...dispatch,
    consumed_tokens: dispatch.consumed_tokens?.map(cloneTraceToken),
    input_items: dispatch.input_items?.map(cloneWorkItemRef),
    output_items: dispatch.output_items?.map(cloneWorkItemRef),
    output_mutations: dispatch.output_mutations?.map(cloneTraceMutation),
    previous_chaining_trace_ids: dispatch.previous_chaining_trace_ids
      ? [...dispatch.previous_chaining_trace_ids]
      : undefined,
    provider_session: dispatch.provider_session
      ? { ...dispatch.provider_session }
      : undefined,
    token_names: dispatch.token_names ? [...dispatch.token_names] : undefined,
    trace_ids: dispatch.trace_ids ? [...dispatch.trace_ids] : undefined,
    work_ids: dispatch.work_ids ? [...dispatch.work_ids] : undefined,
    work_types: dispatch.work_types ? [...dispatch.work_types] : undefined,
  };
}

function cloneTrace(trace: DashboardTrace): DashboardTrace {
  return {
    ...trace,
    dispatches: trace.dispatches.map(cloneTraceDispatch),
    relations: trace.relations?.map((relation) => ({ ...relation })),
    request_ids: trace.request_ids ? [...trace.request_ids] : undefined,
    transition_ids: [...trace.transition_ids],
    work_ids: [...trace.work_ids],
    work_items: trace.work_items?.map(cloneWorkItemRef),
    workstation_sequence: [...trace.workstation_sequence],
  };
}

export function cloneTracesByWorkID(
  tracesByWorkID: Record<string, DashboardTrace>,
): Record<string, DashboardTrace> {
  return Object.fromEntries(
    Object.entries(tracesByWorkID).map(([workID, trace]) => [workID, cloneTrace(trace)]),
  );
}

export function cloneProviderSessionAttempts(
  attempts: DashboardProviderSessionAttempt[],
): DashboardProviderSessionAttempt[] {
  return attempts.map((attempt) => ({
    ...attempt,
    diagnostics: attempt.diagnostics
      ? {
          ...attempt.diagnostics,
          provider: attempt.diagnostics.provider
            ? {
                ...attempt.diagnostics.provider,
                request_metadata: attempt.diagnostics.provider.request_metadata
                  ? { ...attempt.diagnostics.provider.request_metadata }
                  : undefined,
                response_metadata: attempt.diagnostics.provider.response_metadata
                  ? { ...attempt.diagnostics.provider.response_metadata }
                  : undefined,
              }
            : undefined,
          rendered_prompt: attempt.diagnostics.rendered_prompt
            ? {
                ...attempt.diagnostics.rendered_prompt,
                variables: attempt.diagnostics.rendered_prompt.variables
                  ? { ...attempt.diagnostics.rendered_prompt.variables }
                  : undefined,
              }
            : undefined,
        }
      : undefined,
    provider_session: attempt.provider_session
      ? { ...attempt.provider_session }
      : undefined,
    work_items: attempt.work_items?.map(cloneWorkItemRef),
  }));
}

export function cloneFailedWorkDetailsByWorkID(
  failedWorkDetailsByWorkID: Record<string, DashboardFailedWorkDetail>,
): Record<string, DashboardFailedWorkDetail> | undefined {
  const entries = Object.entries(failedWorkDetailsByWorkID).map(([workID, detail]) => [
    workID,
    {
      ...detail,
      work_item: cloneWorkItemRef(detail.work_item),
    },
  ]);

  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

export function cloneWorkRequestsByID(
  workRequestsByID: Record<string, TimelineWorkRequestPayload>,
): Record<string, TimelineWorkRequestPayload> {
  return Object.fromEntries(
    Object.entries(workRequestsByID).map(([requestID, request]) => [
      requestID,
      {
        ...request,
        work_items: request.work_items?.map((item) => ({
          ...item,
          tags: item.tags ? { ...item.tags } : undefined,
        })),
      },
    ]),
  );
}

export function cloneWorkstationDispatchRequestsByID(
  workstationRequestsByDispatchID: Record<string, DashboardWorkstationRequest>,
): Record<string, DashboardWorkstationRequest> {
  return Object.fromEntries(
    Object.entries(workstationRequestsByDispatchID).map(([dispatchID, request]) => [
      dispatchID,
      {
        ...request,
        inference_attempts: request.inference_attempts.map((attempt) => ({ ...attempt })),
        counts: request.counts ? { ...request.counts } : undefined,
        request_metadata: request.request_metadata
          ? { ...request.request_metadata }
          : undefined,
        request_view: cloneRuntimeWorkstationRequestRequest(request.request_view),
        response_metadata: request.response_metadata
          ? { ...request.response_metadata }
          : undefined,
        response_view: cloneRuntimeWorkstationRequestResponse(request.response_view),
        script_request: request.script_request
          ? {
              ...request.script_request,
              args: request.script_request.args ? [...request.script_request.args] : undefined,
            }
          : undefined,
        script_response: request.script_response ? { ...request.script_response } : undefined,
        trace_ids: request.trace_ids ? [...request.trace_ids] : undefined,
        work_items: request.work_items.map(cloneWorkItemRef),
      },
    ]),
  );
}

function cloneRuntimeWorkstationRequestRequest(
  request: DashboardRuntimeWorkstationRequest["request"] | undefined,
): DashboardRuntimeWorkstationRequest["request"] | undefined {
  if (!request) {
    return undefined;
  }

  return {
    ...request,
    consumedTokens: request.consumedTokens?.map((token) => ({
      ...token,
      tags: token.tags ? { ...token.tags } : undefined,
    })),
    inputWorkItems: request.inputWorkItems?.map(cloneWorkItemRef),
    inputWorkTypeIds: request.inputWorkTypeIds ? [...request.inputWorkTypeIds] : undefined,
    requestMetadata: request.requestMetadata ? { ...request.requestMetadata } : undefined,
    traceIds: request.traceIds ? [...request.traceIds] : undefined,
  };
}

function cloneRuntimeWorkstationRequestResponse(
  response: DashboardRuntimeWorkstationRequest["response"] | undefined,
): DashboardRuntimeWorkstationRequest["response"] | undefined {
  if (!response) {
    return undefined;
  }

  const diagnostics = response.diagnostics as
    | (DashboardWorkDiagnostics & {
        provider?: DashboardWorkDiagnostics["provider"] & {
          requestMetadata?: Record<string, string>;
          responseMetadata?: Record<string, string>;
        };
        renderedPrompt?: {
          systemPromptHash?: string;
          userMessageHash?: string;
          variables?: Record<string, string>;
        };
      })
    | undefined;

  return {
    ...response,
    diagnostics: diagnostics
      ? {
          ...diagnostics,
          provider: diagnostics.provider
            ? {
                ...diagnostics.provider,
                requestMetadata: diagnostics.provider.requestMetadata
                  ? { ...diagnostics.provider.requestMetadata }
                  : undefined,
                responseMetadata: diagnostics.provider.responseMetadata
                  ? { ...diagnostics.provider.responseMetadata }
                  : undefined,
              }
            : undefined,
          renderedPrompt: diagnostics.renderedPrompt
            ? {
                ...diagnostics.renderedPrompt,
                variables: diagnostics.renderedPrompt.variables
                  ? { ...diagnostics.renderedPrompt.variables }
                  : undefined,
              }
            : undefined,
        }
      : undefined,
    outputMutations: response.outputMutations?.map((mutation) => ({
      ...mutation,
      resulting_token: mutation.resulting_token
        ? {
            ...mutation.resulting_token,
            tags: mutation.resulting_token.tags
              ? { ...mutation.resulting_token.tags }
              : undefined,
          }
        : undefined,
    })),
    outputWorkItems: response.outputWorkItems?.map(cloneWorkItemRef),
    providerSession: response.providerSession ? { ...response.providerSession } : undefined,
    responseMetadata: response.responseMetadata ? { ...response.responseMetadata } : undefined,
  };
}

export function cloneInferenceAttemptsByDispatchID(
  attemptsByDispatchID: Record<string, Record<string, DashboardInferenceAttempt>>,
): Record<string, Record<string, DashboardInferenceAttempt>> | undefined {
  const entries = Object.entries(attemptsByDispatchID).map(([dispatchID, attempts]) => [
    dispatchID,
    Object.fromEntries(
      Object.entries(attempts).map(([requestID, attempt]) => [
        requestID,
        {
          ...attempt,
          diagnostics: attempt.diagnostics
            ? {
                ...attempt.diagnostics,
                provider: attempt.diagnostics.provider
                  ? {
                      ...attempt.diagnostics.provider,
                      request_metadata: attempt.diagnostics.provider.request_metadata
                        ? { ...attempt.diagnostics.provider.request_metadata }
                        : undefined,
                      response_metadata: attempt.diagnostics.provider.response_metadata
                        ? { ...attempt.diagnostics.provider.response_metadata }
                        : undefined,
                    }
                  : undefined,
                rendered_prompt: attempt.diagnostics.rendered_prompt
                  ? {
                      ...attempt.diagnostics.rendered_prompt,
                      variables: attempt.diagnostics.rendered_prompt.variables
                        ? { ...attempt.diagnostics.rendered_prompt.variables }
                        : undefined,
                    }
                  : undefined,
              }
            : undefined,
          provider_session: attempt.provider_session ? { ...attempt.provider_session } : undefined,
        },
      ]),
    ),
  ] as const);

  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}
