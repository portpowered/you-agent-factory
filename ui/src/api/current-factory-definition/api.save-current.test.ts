// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: save-current coverage shares one API fixture seam split from the oversized api.test.ts suite.
import {
  factoryRuntimeNotIdleTarget,
  staleFactoryVersionTarget,
} from "../../testing/factory-validation-target-fixtures";
import { sessionFactoryOperatorErrorMessages } from "../session-factory/operator-errors";
import { saveCurrentFactoryDocument } from "./api";

describe("saveCurrentFactoryDocument", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("saves the editable current-factory definition with version metadata", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "10",
            physical: "2026-05-18T14:40:00Z",
          },
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

    const document = await saveCurrentFactoryDocument(
      {
        baseVersion: {
          logical: "9",
          physical: "2026-05-18T14:25:00Z",
        },
        factoryDefinition: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:25:00.001Z",
            },
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      }),
    );
    expect(document.version.logical).toBe("10");
  });

  it("increments large logical version strings without losing precision before save", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "1779941481569583434",
            physical: "2026-05-28T04:11:21.569583434Z",
          },
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

    await saveCurrentFactoryDocument(
      {
        baseVersion: {
          logical: "1779941481569583433",
          physical: "2026-05-28T04:11:21.569583433Z",
        },
        factoryDefinition: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
      },
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: expect.stringContaining('"factory":{"name":"Current Factory"'),
      }),
    );
    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: expect.stringContaining('"logical":"1779941481569583434"'),
      }),
    );
    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: expect.stringContaining('"physical":"2026-05-28T04:11:21.570Z"'),
      }),
    );
  });

  it("never sends UPSERT_NAMED_AND_ACTIVATE on editor save paths", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "10",
            physical: "2026-05-18T14:40:00Z",
          },
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

    await saveCurrentFactoryDocument(
      {
        factoryDefinition: {
          name: "Current Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
      },
      { fetch },
    );

    const putBody = JSON.parse(
      String(
        vi
          .mocked(fetch)
          .mock.calls.find(([, init]) => init?.method === "PUT")?.[1]?.body,
      ),
    ) as { mode?: string };

    expect(putBody.mode).toBe("REPLACE_CURRENT");
    expect(putBody.mode).not.toBe("UPSERT_NAMED_AND_ACTIVATE");
  });

  it("saves through the session-scoped editable-definition route for non-default sessions", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Scoped Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "11",
            physical: "2026-05-18T14:41:00Z",
          },
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

    await saveCurrentFactoryDocument(
      {
        factoryDefinition: {
          name: "Scoped Factory",
          workers: [],
          workstations: [],
          workTypes: [],
        },
      },
      {
        fetch,
        sessionID: "session-2",
      },
    );

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        method: "PUT",
      }),
    );
  });

  it("preserves structured save error targets when the API rejects a topology edit", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          baseVersion: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "STALE_FACTORY_VERSION",
                message: "The editable definition is stale.",
                targets: [
                  staleFactoryVersionTarget(
                    "The editable definition is stale.",
                  ),
                ],
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      status: 409,
      targets: [staleFactoryVersionTarget("The editable definition is stale.")],
    });
  });

  it("preserves valid structured save error targets from mixed target arrays", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          baseVersion: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "STALE_FACTORY_VERSION",
                message: "The editable definition is stale.",
                targets: [
                  staleFactoryVersionTarget(
                    "The editable definition is stale.",
                  ),
                  "invalid-target-entry",
                ],
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      status: 409,
      targets: [staleFactoryVersionTarget("The editable definition is stale.")],
    });
  });

  it("preserves active-work save rejections from the editable current-factory API", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          baseVersion: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "FACTORY_NOT_IDLE",
                message:
                  "Current factory runtime must be idle before activation.",
                targets: [
                  factoryRuntimeNotIdleTarget(
                    "Current factory runtime must be idle before activation.",
                  ),
                ],
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
      status: 409,
      targets: [
        factoryRuntimeNotIdleTarget(
          "Current factory runtime must be idle before activation.",
        ),
      ],
    });
  });

  it("surfaces the existing network fallback when the editable-definition save request throws", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockRejectedValue(new Error("socket closed")),
        },
      ),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: "The dashboard could not reach the current factory editing API.",
      name: "CurrentFactoryDefinitionError",
      responseBody: expect.any(Error),
    });
  });

  it("keeps the existing save rejection fallback when the API does not return a structured error message", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response("", {
              status: 500,
              statusText: "Internal Server Error",
            }),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "The current factory editing API rejected the save request.",
      name: "CurrentFactoryDefinitionError",
      responseBody: null,
      status: 500,
      statusText: "Internal Server Error",
    });
  });

  it("rejects save success payloads whose factory definition cannot be normalized canonically", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                name: "Current Factory",
                workers: [
                  {
                    model: 42,
                    name: "writer",
                    type: "MODEL_WORKER",
                  },
                ],
                workstations: [],
                workTypes: [],
                version: {
                  logical: "12",
                  physical: "2026-05-18T14:30:00Z",
                },
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 200,
                statusText: "OK",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY_DEFINITION",
      message:
        "The current factory editing API returned a factory definition the dashboard cannot edit. factory.workers[0].model must be a string.",
      name: "CurrentFactoryDefinitionError",
      responseBody: {
        name: "Current Factory",
        workers: [
          {
            model: 42,
            name: "writer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
        workTypes: [],
        version: {
          logical: "12",
          physical: "2026-05-18T14:30:00Z",
        },
      },
      status: 200,
      statusText: "OK",
    });
  });

  it("surfaces session-factory operator copy for stale-version save failures", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          baseVersion: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "STALE_FACTORY_VERSION",
                message: "ignored api message",
                targets: [staleFactoryVersionTarget()],
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      message: sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
      name: "CurrentFactoryDefinitionError",
    });
  });

  it("surfaces session-factory operator copy for not-idle save failures", async () => {
    await expect(
      saveCurrentFactoryDocument(
        {
          baseVersion: {
            logical: "9",
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "FACTORY_NOT_IDLE",
                message: "ignored api message",
                targets: [factoryRuntimeNotIdleTarget()],
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
      message: sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
      name: "CurrentFactoryDefinitionError",
    });
  });
});
