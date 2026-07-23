import { describe, expect, it } from "vitest";

import type { components, operations, paths } from "./openapi";

type FactoryLayoutEmptyState =
  components["schemas"]["FactoryLayoutEmptyState"];
type ProviderCatalog = components["schemas"]["ProviderCatalog"];

describe("generated session factory OpenAPI types", () => {
  it("round-trips the typed provider catalog contract without changing meaning", () => {
    const catalog: ProviderCatalog = {
      formatVersion: "1.0.0",
      providers: [
        {
          id: "example-alpha",
          aliases: ["alpha"],
          displayName: {
            type: "LOCALIZABLE_ASSET",
            value: "Example Alpha",
          },
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Example provider metadata.",
          },
          documentation: [
            {
              kind: "reference",
              url: "https://example.com/providers/alpha",
            },
          ],
          technicalSupportLevel: "experimental",
          implementationAvailability: "externally-supplied",
          maximumExecutionCapabilities: {
            promptSubmission: true,
            imageInput: false,
            sessionResume: true,
            structuredOutput: true,
            toolExecution: true,
            workingDirectory: true,
            worktree: false,
          },
          maximumResponseFidelityCapabilities: {
            nativeStreaming: true,
            messageDeltas: true,
            messageSnapshots: true,
            reasoningSummaries: false,
            toolLifecycle: true,
            toolOutputDeltas: false,
            fileChanges: false,
            plans: false,
            usage: true,
            stableItemIds: true,
            providerReconnect: true,
          },
          discovery: {
            executableNames: ["example-alpha"],
            endpointKinds: ["stdio"],
            configurationKeys: ["providers.example-alpha.enabled"],
          },
          deprecation: {
            deprecatedSince: "2026-07-23",
            reason: {
              type: "LOCALIZABLE_ASSET",
              value: "Use Example Next for new integrations.",
            },
            replacementProviderId: "example-next",
          },
        },
      ],
    };

    const roundTrip: ProviderCatalog = JSON.parse(JSON.stringify(catalog));
    expect(roundTrip.providers[0]).toMatchObject({
      id: "example-alpha",
      technicalSupportLevel: "experimental",
      implementationAvailability: "externally-supplied",
      deprecation: {
        replacementProviderId: "example-next",
      },
    });
  });

  it("exposes concrete text and image empty-state variants", () => {
    const textState: FactoryLayoutEmptyState = {
      text: "No work is waiting.",
    };
    const imageState: FactoryLayoutEmptyState = {
      image: {
        alternativeText: "No active review",
        source: {
          data: "AQID",
          kind: "EMBEDDED",
          mediaType: "image/png",
        },
      },
    };

    expect(textState.text).toBe("No work is waiting.");
    expect(imageState.image.alternativeText).toBe("No active review");
  });

  it("supports a typed response-event stream consumer", () => {
    type ResponseEventOperation =
      operations["getFactoryResponseEventsBySessionId"];

    const request: ResponseEventOperation["parameters"] = {
      path: { session_id: "session one" },
      query: {
        after_sequence: 42,
        dispatch_id: "dispatch/one",
        kind: ["MESSAGE", "TOOL"],
      },
    };
    const route: paths["/factory-sessions/{session_id}/response-events"]["get"] =
      {} as ResponseEventOperation;
    const success: ResponseEventOperation["responses"][200]["content"]["text/event-stream"] =
      "data: {}\n\n";
    const badRequest: ResponseEventOperation["responses"][400]["content"]["application/json"] =
      {
        code: "INVALID_RESPONSE_EVENT_CURSOR",
        family: "BAD_REQUEST",
        message: "response-event cursor is invalid",
      };
    const notFound: ResponseEventOperation["responses"][404]["content"]["application/json"] =
      {
        code: "RESPONSE_EVENT_SESSION_NOT_FOUND",
        family: "NOT_FOUND",
        message: "factory session not found",
      };
    const expired: ResponseEventOperation["responses"][410]["content"]["application/json"] =
      {
        code: "RESPONSE_EVENT_STREAM_EXPIRED",
        family: "GONE",
        message: "response-event stream expired",
      };
    const internalError: ResponseEventOperation["responses"][500]["content"]["application/json"] =
      {
        code: "INTERNAL_ERROR",
        family: "INTERNAL_SERVER_ERROR",
        message: "unexpected response-event stream failure",
      };

    expect(route).toEqual({});
    expect(request).toEqual({
      path: { session_id: "session one" },
      query: {
        after_sequence: 42,
        dispatch_id: "dispatch/one",
        kind: ["MESSAGE", "TOOL"],
      },
    });
    expect(success).toBe("data: {}\n\n");
    expect([
      badRequest.code,
      notFound.code,
      expired.code,
      internalError.code,
    ]).toEqual([
      "INVALID_RESPONSE_EVENT_CURSOR",
      "RESPONSE_EVENT_SESSION_NOT_FOUND",
      "RESPONSE_EVENT_STREAM_EXPIRED",
      "INTERNAL_ERROR",
    ]);
  });

  it("exposes typed session factory save payloads and machine-readable error codes", () => {
    const factory: components["schemas"]["Factory"] = {
      name: "customer-support-triage",
      workTypes: [
        {
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workers: [
        {
          executorProvider: "SCRIPT_WRAP",
          model: "claude-sonnet-4-20250514",
          modelProvider: "CLAUDE",
          name: "planner",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          inputs: [{ state: "init", workType: "task" }],
          name: "plan-task",
          onContinue: [
            { state: "init", workType: "task" },
            { state: "queued", workType: "task" },
          ],
          onRejection: [
            { state: "rejected", workType: "task" },
            { state: "backlog", workType: "task" },
          ],
          onFailure: [
            { state: "failed", workType: "task" },
            { state: "blocked", workType: "task" },
          ],
          outputs: [{ state: "done", workType: "task" }],
          worker: "planner",
        },
      ],
    };
    const saveRequest: operations["saveCurrentFactoryBySessionId"]["requestBody"]["content"]["application/json"] =
      {
        mode: "REPLACE_CURRENT",
        factory,
      };
    const current: paths["/factory-sessions/{session_id}/factory"]["get"]["responses"][200]["content"]["application/json"] =
      factory;
    const invalidName: components["schemas"]["ErrorResponse"]["code"] =
      "INVALID_FACTORY_NAME";
    const configLoadFailed: components["schemas"]["ErrorResponse"]["code"] =
      "FACTORY_SESSION_CONFIG_LOAD_FAILED";
    const invalidFactory: components["schemas"]["ErrorResponse"]["code"] =
      "INVALID_FACTORY";
    const duplicateName: components["schemas"]["ErrorResponse"]["code"] =
      "FACTORY_ALREADY_EXISTS";
    const runtimeBusy: components["schemas"]["ErrorResponse"]["code"] =
      "FACTORY_NOT_IDLE";
    const badRequestFamily: components["schemas"]["ErrorResponse"]["family"] =
      "BAD_REQUEST";
    const conflictFamily: components["schemas"]["ErrorResponse"]["family"] =
      "CONFLICT";
    const notFoundFamily: components["schemas"]["ErrorResponse"]["family"] =
      "NOT_FOUND";
    const currentNotFound: paths["/factory-sessions/{session_id}/factory"]["get"]["responses"][404]["content"]["application/json"] =
      {
        code: "NOT_FOUND",
        family: "NOT_FOUND",
        message: "factory session not found",
      };

    expect(saveRequest.mode).toBe("REPLACE_CURRENT");
    expect(saveRequest.factory.name).toBe("customer-support-triage");
    expect(current.name).toBe("customer-support-triage");
    expect(current.workstations?.[0]?.worker).toBe("planner");
    expect(currentNotFound.code).toBe("NOT_FOUND");
    expect(currentNotFound.family).toBe("NOT_FOUND");
    expect([
      invalidName,
      configLoadFailed,
      invalidFactory,
      duplicateName,
      runtimeBusy,
    ]).toEqual([
      "INVALID_FACTORY_NAME",
      "FACTORY_SESSION_CONFIG_LOAD_FAILED",
      "INVALID_FACTORY",
      "FACTORY_ALREADY_EXISTS",
      "FACTORY_NOT_IDLE",
    ]);
    expect([badRequestFamily, conflictFamily, notFoundFamily]).toEqual([
      "BAD_REQUEST",
      "CONFLICT",
      "NOT_FOUND",
    ]);
  });
});
