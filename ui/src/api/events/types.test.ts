import type {
  FactoryEvent,
  InferenceRequestPayload,
  InferenceResponsePayload,
  RunResponsePayload,
  ScriptRequestPayload,
  ScriptResponsePayload,
} from "./types";
import { FACTORY_EVENT_TYPES } from "./types";

const eventTime = "2026-04-18T12:30:00Z";

describe("factory event types", () => {
  it("exposes typed inference request and response payloads", () => {
    const request = inferenceEvent(
      "event-inference-request",
      FACTORY_EVENT_TYPES.inferenceRequest,
      "dispatch-1",
      {
        attempt: 1,
        inferenceRequestId: "inference-request-1",
        prompt: "Draft release notes for the event log.",
        workingDirectory: "/tmp/factory/work",
        worktree: "/tmp/factory/worktree",
      },
    );
    const response = inferenceEvent(
      "event-inference-response",
      FACTORY_EVENT_TYPES.inferenceResponse,
      "dispatch-1",
      {
        attempt: 1,
        durationMillis: 124,
        inferenceRequestId: "inference-request-1",
        outcome: "SUCCEEDED",
        response: "Release notes drafted.",
      },
    );

    expect(request.type).toBe("INFERENCE_REQUEST");
    expect(response.type).toBe("INFERENCE_RESPONSE");
    expect(request.context.dispatchId).toBe("dispatch-1");
    expect(response.payload.inferenceRequestId).toBe(request.payload.inferenceRequestId);
  });

  it("includes the canonical run response payload in the maintained event union", () => {
    const response: FactoryEvent<RunResponsePayload> = {
      context: {
        eventTime,
        sequence: 2,
        tick: 2,
      },
      id: "event-run-response",
      payload: {
        reason: "all work finished",
        state: "COMPLETED",
      },
      schemaVersion: "agent-factory.event.v1",
      type: FACTORY_EVENT_TYPES.runResponse,
    };

    expect(response.type).toBe("RUN_RESPONSE");
    expect(response.payload.state).toBe("COMPLETED");
  });

  it("exposes typed script request and response payloads", () => {
    const request = scriptEvent("event-script-request", FACTORY_EVENT_TYPES.scriptRequest, {
      args: ["--work", "work-1", "--project", "docs"],
      attempt: 1,
      command: "script-tool",
      dispatchId: "dispatch-script-1",
      scriptRequestId: "script-request-1",
      transitionId: "transition-script-1",
    });
    const response = scriptEvent("event-script-response", FACTORY_EVENT_TYPES.scriptResponse, {
      attempt: 1,
      dispatchId: "dispatch-script-1",
      durationMillis: 238,
      exitCode: 3,
      outcome: "FAILED_EXIT_CODE",
      scriptRequestId: "script-request-1",
      stderr: "script stderr\n",
      stdout: "script stdout\n",
      transitionId: "transition-script-1",
    });

    expect(request.type).toBe("SCRIPT_REQUEST");
    expect(response.type).toBe("SCRIPT_RESPONSE");
    expect(response.payload.scriptRequestId).toBe(request.payload.scriptRequestId);
  });
});

function inferenceEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.inferenceRequest,
  dispatchId: string,
  payload: InferenceRequestPayload,
): FactoryEvent<InferenceRequestPayload>;
function inferenceEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.inferenceResponse,
  dispatchId: string,
  payload: InferenceResponsePayload,
): FactoryEvent<InferenceResponsePayload>;
function inferenceEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.inferenceRequest | typeof FACTORY_EVENT_TYPES.inferenceResponse,
  dispatchId: string,
  payload: InferenceRequestPayload | InferenceResponsePayload,
): FactoryEvent<InferenceRequestPayload | InferenceResponsePayload> {
  return {
    context: {
      dispatchId,
      eventTime,
      sequence: 1,
      tick: 1,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  };
}

function scriptEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.scriptRequest,
  payload: ScriptRequestPayload,
): FactoryEvent<ScriptRequestPayload>;
function scriptEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.scriptResponse,
  payload: ScriptResponsePayload,
): FactoryEvent<ScriptResponsePayload>;
function scriptEvent(
  id: string,
  type: typeof FACTORY_EVENT_TYPES.scriptRequest | typeof FACTORY_EVENT_TYPES.scriptResponse,
  payload: ScriptRequestPayload | ScriptResponsePayload,
): FactoryEvent<ScriptRequestPayload | ScriptResponsePayload> {
  return {
    context: {
      dispatchId: payload.dispatchId,
      eventTime,
      sequence: 3,
      tick: 2,
    },
    id,
    payload,
    schemaVersion: "agent-factory.event.v1",
    type,
  };
}

