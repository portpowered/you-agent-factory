import {
  factorySessionFieldTarget,
  factorySessionTargetTarget,
} from "../../testing/factory-validation-target-fixtures";
import {
  closeFactorySession,
  FactorySessionsAPIError,
  getFactorySession,
  getFactorySessionArtifact,
  getFactorySessionPartialResult,
  getFactorySessionResult,
  listFactorySessions,
  openFactorySession,
} from "./api";
import {
  getFactorySessionDurableResults,
  listFactorySessionDispatches,
} from "./api-durable-inspection";
import { getFactorySessionDispatchDetail } from "./dispatch-detail";
import { getFactorySessionSyncPreflight } from "./sync-preflight";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("factory sessions API", () => {
  it("lists live factory sessions from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessions: [
            {
              factoryDir: "/workspace/project/alpha",
              folderPath: "/workspace/project",
              id: "019e0000-0000-7000-8000-000000000042",
              isDefault: true,
              project: "alpha",
              runtime: {
                dispatches: [{ id: "dispatch-legacy" }],
                orchestratorKind: "PETRI_NET",
              },
              target: {
                kind: "default",
              },
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listFactorySessions()).resolves.toEqual([
      {
        factoryDir: "/workspace/project/alpha",
        folderPath: "/workspace/project",
        id: "019e0000-0000-7000-8000-000000000042",
        isDefault: true,
        project: "alpha",
        runtime: {
          orchestratorKind: "PETRI_NET",
        },
        target: {
          kind: "default",
        },
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({ method: "GET" }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("posts folder and target selection when opening a factory session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          session: {
            factoryDir: "/workspace/project/beta",
            folderPath: "/workspace/project",
            id: "session-beta",
            isDefault: false,
            project: "beta",
            runtime: {
              dispatches: [{ id: "dispatch-legacy" }],
              orchestratorKind: "JAVASCRIPT",
            },
            target: {
              kind: "named",
              name: "beta",
            },
          },
          targets: [
            {
              factoryDir: "/workspace/project/beta",
              ref: {
                kind: "named",
                name: "beta",
              },
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
        target: {
          kind: "named",
          name: "beta",
        },
      }),
    ).resolves.toEqual({
      session: {
        factoryDir: "/workspace/project/beta",
        folderPath: "/workspace/project",
        id: "session-beta",
        isDefault: false,
        project: "beta",
        runtime: {
          orchestratorKind: "JAVASCRIPT",
        },
        target: {
          kind: "named",
          name: "beta",
        },
      },
      targets: [
        {
          factoryDir: "/workspace/project/beta",
          ref: {
            kind: "named",
            name: "beta",
          },
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
          target: {
            kind: "named",
            name: "beta",
          },
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("posts validateOnly when checking a folder before launch", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          targets: [
            {
              factoryDir: "/workspace/project",
              folderPath: "/workspace/project",
              label: "default",
              project: "project",
              ref: {
                kind: "default",
              },
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
        validateOnly: true,
      }),
    ).resolves.toEqual({
      targets: [
        {
          factoryDir: "/workspace/project",
          folderPath: "/workspace/project",
          label: "default",
          project: "project",
          ref: {
            kind: "default",
          },
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
          validateOnly: true,
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts initNewFactory when creating a factory from an empty folder", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          session: {
            factoryDir: "/workspace/new",
            folderPath: "/workspace/new",
            id: "session-new",
            isDefault: false,
            project: "new",
            target: {
              kind: "default",
            },
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/new",
        initNewFactory: true,
      }),
    ).resolves.toEqual({
      session: {
        factoryDir: "/workspace/new",
        folderPath: "/workspace/new",
        id: "session-new",
        isDefault: false,
        project: "new",
        target: {
          kind: "default",
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/new",
          initNewFactory: true,
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("loads durable artifact detail from the typed artifact route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessionId: "dur-sess-js-success-002",
          id: "art-js-success-001",
          kind: "FINAL_RESULT",
          visibility: "PUBLIC",
          label: "Docs refresh output",
          content: [
            {
              text: "Documentation refresh complete.",
              type: "text",
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionArtifact(
        "dur-sess-js-success-002",
        "art-js-success-001",
      ),
    ).resolves.toMatchObject({
      content: [
        {
          text: "Documentation refresh complete.",
          type: "text",
        },
      ],
      id: "art-js-success-001",
      sessionId: "dur-sess-js-success-002",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-success-002/artifacts/art-js-success-001",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("reads the sync preflight surface with reconnect cursor query parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          backendScopeId: "backend-a",
          checkpointReusable: true,
          factorySessionId: "session-beta",
          logicalSessionKeyId: "logical-beta",
          reasonCode: "ok",
          reconnectCursor: {
            afterEventId: "event-7",
            afterSequence: 7,
            provided: true,
            validForStreamGeneration: true,
          },
          requestedSessionId: "session-beta",
          streamGenerationId: "stream-beta",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionSyncPreflight("session-beta", {
        afterEventId: "event-7",
        afterSequence: 7,
      }),
    ).resolves.toEqual({
      backendScopeId: "backend-a",
      checkpointReusable: true,
      factorySessionId: "session-beta",
      logicalSessionKeyId: "logical-beta",
      reasonCode: "ok",
      reconnectCursor: {
        afterEventId: "event-7",
        afterSequence: 7,
        provided: true,
        validForStreamGeneration: true,
      },
      requestedSessionId: "session-beta",
      streamGenerationId: "stream-beta",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/sync-preflight?after_event_id=event-7&after_sequence=7",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("reads the sync preflight surface with logical identity hints", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          backendScopeId: "backend-a",
          checkpointReusable: true,
          factorySessionId: "session-remapped",
          logicalSessionKeyId: "lsk-named-target",
          reasonCode: "logical_session_remap",
          reconnectCursor: {
            provided: false,
            validForStreamGeneration: false,
          },
          requestedSessionId: "session-stale",
          streamGenerationId: "stream-beta",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionSyncPreflight(
        "session-stale",
        {
          afterEventId: "event-7",
          afterSequence: 7,
        },
        {
          backendScopeId: "backend-a",
          logicalSessionKeyId: "lsk-named-target",
        },
      ),
    ).resolves.toMatchObject({
      factorySessionId: "session-remapped",
      reasonCode: "logical_session_remap",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-stale/sync-preflight?after_event_id=event-7&after_sequence=7&backend_scope_id=backend-a&logical_session_key_id=lsk-named-target",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("rejects invalid durable artifact detail responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id: "art-js-success-001" }), {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        }),
      ),
    );

    await expect(
      getFactorySessionArtifact(
        "dur-sess-js-success-002",
        "art-js-success-001",
      ),
    ).rejects.toMatchObject<Partial<FactorySessionsAPIError>>({
      code: "INTERNAL_ERROR",
      message: "The factory sessions API returned an invalid response.",
    });
  });

  it("rejects partial resumable sync preflight responses that omit the identity set", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          checkpointReusable: true,
          reasonCode: "ok",
          reconnectCursor: {
            provided: false,
            validForStreamGeneration: true,
          },
          requestedSessionId: "session-beta",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionSyncPreflight("session-beta"),
    ).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API returned an invalid response.",
        {
          code: "INTERNAL_ERROR",
          responseBody: {
            checkpointReusable: true,
            reasonCode: "ok",
            reconnectCursor: {
              provided: false,
              validForStreamGeneration: true,
            },
            requestedSessionId: "session-beta",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );
  });

  it("accepts validateOnly initsNewFactory discovery responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          folderPath: "/workspace/empty",
          initsNewFactory: true,
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/empty",
        validateOnly: true,
      }),
    ).resolves.toEqual({
      folderPath: "/workspace/empty",
      initsNewFactory: true,
    });
  });

  it("maps validation failures into typed API errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "factory session folder is required",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "",
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError("factory session folder is required", {
        code: "BAD_REQUEST",
        responseBody: {
          code: "BAD_REQUEST",
          message: "factory session folder is required",
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("preserves structured validation targets on typed API errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "folder validation failed",
          targets: [
            factorySessionFieldTarget(
              "missing",
              "folderPath",
              "folder validation failed",
            ),
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/missing",
        validateOnly: true,
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError("folder validation failed", {
        code: "BAD_REQUEST",
        responseBody: {
          code: "BAD_REQUEST",
          message: "folder validation failed",
          targets: [
            factorySessionFieldTarget(
              "missing",
              "folderPath",
              "folder validation failed",
            ),
          ],
        },
        status: 400,
        statusText: "Bad Request",
        targets: [
          factorySessionFieldTarget(
            "missing",
            "folderPath",
            "folder validation failed",
          ),
        ],
      }),
    );
  });

  it("preserves config-load-failed responses as distinct typed API errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
          message:
            "factory configuration could not be loaded from the selected folder",
          targets: [
            factorySessionTargetTarget(
              "config_load_failed",
              "default",
              'Factory target "default" at "/workspace/project" could not be loaded: unexpected end of JSON input',
            ),
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
        validateOnly: true,
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError(
        "factory configuration could not be loaded from the selected folder",
        {
          code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
          responseBody: {
            code: "FACTORY_SESSION_CONFIG_LOAD_FAILED",
            message:
              "factory configuration could not be loaded from the selected folder",
            targets: [
              factorySessionTargetTarget(
                "config_load_failed",
                "default",
                'Factory target "default" at "/workspace/project" could not be loaded: unexpected end of JSON input',
              ),
            ],
          },
          status: 400,
          statusText: "Bad Request",
          targets: [
            factorySessionTargetTarget(
              "config_load_failed",
              "default",
              'Factory target "default" at "/workspace/project" could not be loaded: unexpected end of JSON input',
            ),
          ],
        },
      ),
    );
  });

  it("keeps the list fallback error for empty error bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 503,
        statusText: "Service Unavailable",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listFactorySessions()).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: null,
          status: 503,
          statusText: "Service Unavailable",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("keeps the open fallback error for malformed JSON bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("{", {
        headers: {
          "Content-Type": "application/json",
        },
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: "{",
          status: 500,
          statusText: "Internal Server Error",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("keeps the close fallback error for plain-text error bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("session close failed upstream", {
        status: 502,
        statusText: "Bad Gateway",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(closeFactorySession("session-beta")).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: "session close failed upstream",
          status: 502,
          statusText: "Bad Gateway",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});

describe("factory sessions read and durable API", () => {
  it("loads one live factory session from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "session-beta",
          runtime: {
            dispatches: [{ id: "dispatch-legacy" }],
            orchestratorKind: "JAVASCRIPT",
            streamGenerationID: "stream-gen-live-001",
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getFactorySession("session-beta")).resolves.toEqual({
      session: {
        id: "session-beta",
        runtime: {
          orchestratorKind: "JAVASCRIPT",
          streamGenerationID: "stream-gen-live-001",
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta",
      expect.objectContaining({ method: "GET" }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("normalizes durable factory session reads into shared FactorySession runtime shape", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dialect: "you-workflow-v1",
          lifecycle: {
            startedAt: "2026-06-08T14:00:00Z",
            updatedAt: "2026-06-08T14:05:00Z",
          },
          orchestratorKind: "JAVASCRIPT",
          phase: "verify",
          progress: {
            completedDispatches: 1,
            failedDispatches: 0,
            inFlightDispatches: 1,
            totalDispatches: 3,
          },
          resolvedSource: {
            kind: "WORKFLOW_NAME",
            sourceRef: "workflow/release-train",
            sourceHash: "sha256:js-workflow-release-train",
          },
          sessionId: "dur-sess-js-run-n-001",
          status: "RUNNING",
          usage: { resources: [] },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getFactorySession("dur-sess-js-run-n-001")).resolves.toEqual({
      durableLifecycleStatus: "RUNNING",
      durableProgress: {
        completedDispatches: 1,
        failedDispatches: 0,
        inFlightDispatches: 1,
        totalDispatches: 3,
      },
      durableReadModel: expect.objectContaining({
        dialect: "you-workflow-v1",
        orchestratorKind: "JAVASCRIPT",
        phase: "verify",
        resolvedSource: {
          kind: "WORKFLOW_NAME",
          sourceHash: "sha256:js-workflow-release-train",
          sourceRef: "workflow/release-train",
        },
        sessionId: "dur-sess-js-run-n-001",
        status: "RUNNING",
      }),
      partialResult: undefined,
      result: undefined,
      resultSummary: undefined,
      session: expect.objectContaining({
        id: "dur-sess-js-run-n-001",
        runtime: expect.objectContaining({
          javascript: expect.objectContaining({
            phase: "verify",
            scriptStatus: "RUNNING",
          }),
          orchestratorKind: "JAVASCRIPT",
          status: "ACTIVE",
        }),
      }),
    });
  });

  it("loads live terminal and partial result surfaces from the typed API", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessionId: "session-beta",
            status: "SUCCEEDED",
            result: [{ type: "text", text: "done" }],
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessionId: "session-beta",
            status: "RUNNING",
            result: [{ type: "text", text: "partial" }],
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(getFactorySessionResult("session-beta")).resolves.toEqual({
      sessionId: "session-beta",
      status: "SUCCEEDED",
      result: [{ type: "text", text: "done" }],
    });
    await expect(
      getFactorySessionPartialResult("session-beta"),
    ).resolves.toEqual({
      sessionId: "session-beta",
      status: "RUNNING",
      result: [{ type: "text", text: "partial" }],
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/factory-sessions/session-beta/result",
      expect.objectContaining({ method: "GET" }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/session-beta/partial-result",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("lists durable factory session dispatches from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dispatches: [
            {
              attempt: 1,
              dispatchKind: "JAVASCRIPT_VERIFY",
              id: "disp-js-success-002",
              label: "verify-docs",
              status: "COMPLETED",
            },
          ],
          sessionId: "dur-sess-js-success-002",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      listFactorySessionDispatches("dur-sess-js-success-002"),
    ).resolves.toEqual({
      dispatches: [
        {
          attempt: 1,
          dispatchKind: "JAVASCRIPT_VERIFY",
          id: "disp-js-success-002",
          label: "verify-docs",
          status: "COMPLETED",
        },
      ],
      sessionId: "dur-sess-js-success-002",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-success-002/dispatches",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("loads durable factory session results from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          artifactRefs: [
            {
              id: "art-js-success-001",
              kind: "FINAL_RESULT",
              visibility: "PUBLIC",
            },
          ],
          mode: "final",
          resultStatus: "FINAL",
          sessionId: "dur-sess-js-success-002",
          sessionStatus: "SUCCEEDED",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionDurableResults("dur-sess-js-success-002", "final"),
    ).resolves.toEqual({
      artifactRefs: [
        {
          id: "art-js-success-001",
          kind: "FINAL_RESULT",
          visibility: "PUBLIC",
        },
      ],
      mode: "final",
      resultStatus: "FINAL",
      sessionId: "dur-sess-js-success-002",
      sessionStatus: "SUCCEEDED",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/dur-sess-js-success-002/results?mode=final",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("loads one durable dispatch detail from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          artifactIds: ["artifact-1"],
          dispatchKind: "JAVASCRIPT_VERIFY",
          id: "dispatch-1",
          javascript: {
            executionMode: "live",
            taskKind: "VERIFY",
            taskLabel: "verify release",
          },
          orchestratorKind: "JAVASCRIPT",
          providerSessionRefs: [
            {
              id: "sess-1",
              kind: "session_id",
              provider: "codex",
            },
          ],
          sessionId: "session-beta",
          status: "FAILED",
          statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionDispatchDetail({
        dispatch_id: "dispatch-1",
        session_id: "session-beta",
      }),
    ).resolves.toEqual({
      artifactIds: ["artifact-1"],
      dispatchKind: "JAVASCRIPT_VERIFY",
      id: "dispatch-1",
      javascript: {
        executionMode: "live",
        taskKind: "VERIFY",
        taskLabel: "verify release",
      },
      orchestratorKind: "JAVASCRIPT",
      providerSessionRefs: [
        {
          id: "sess-1",
          kind: "session_id",
          provider: "codex",
        },
      ],
      sessionId: "session-beta",
      status: "FAILED",
      statusTransitions: ["QUEUED", "RUNNING", "FAILED"],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/dispatches/dispatch-1",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("rejects invalid durable dispatch detail responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          dispatchKind: "JAVASCRIPT_VERIFY",
          id: "dispatch-1",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      getFactorySessionDispatchDetail({
        dispatch_id: "dispatch-1",
        session_id: "session-beta",
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API returned an invalid response.",
        {
          code: "INTERNAL_ERROR",
          responseBody: {
            dispatchKind: "JAVASCRIPT_VERIFY",
            id: "dispatch-1",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );
  });
  it("deletes one live factory session from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 204,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(closeFactorySession("session-beta")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});
