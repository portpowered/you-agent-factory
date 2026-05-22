import {
  CurrentEditableFactoryDefinitionError,
  getCurrentEditableFactoryDefinition,
  getCurrentEditableFactoryDefinitionDocument,
  saveCurrentEditableFactoryDefinitionDocument,
} from "./api";

describe("getCurrentEditableFactoryDefinition", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads the current factory through the existing API and preserves editable workstation fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factoryDefinition: {
            id: "factory-current",
            name: "Current Factory",
            workers: [
              {
                model: "gpt-5",
                name: "writer",
                type: "MODEL_WORKER",
              },
            ],
            workstations: [
              {
                body: "Summarize the work item before review.",
                inputs: [
                  {
                    state: "queued",
                    workType: "task",
                  },
                ],
                name: "Draft",
                outputs: [
                  {
                    state: "reviewed",
                    workType: "task",
                  },
                ],
                promptFile: "prompts/draft.md",
                type: "MODEL_WORKSTATION",
                worker: "writer",
              },
            ],
            workTypes: [],
          },
          version: {
            logical: 7,
            physical: "2026-05-18T14:22:00Z",
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

    const factoryDefinition = await getCurrentEditableFactoryDefinition({
      fetch: fetchMock,
    });

    expect(factoryDefinition).toEqual({
      id: "factory-current",
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5",
          name: "writer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Summarize the work item before review.",
          inputs: [
            {
              state: "queued",
              workType: "task",
            },
          ],
          name: "Draft",
          outputs: [
            {
              state: "reviewed",
              workType: "task",
            },
          ],
          promptFile: "prompts/draft.md",
          type: "MODEL_WORKSTATION",
          worker: "writer",
        },
      ],
      workTypes: [],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factories/~default/factory/~current/editable-definition",
      {
        method: "GET",
      },
    );
  });

  it("returns version metadata together with the editable current factory definition document", async () => {
    const document = await getCurrentEditableFactoryDefinitionDocument({
      fetch: vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            factoryDefinition: {
              name: "Current Factory",
              workers: [],
              workstations: [],
              workTypes: [],
            },
            version: {
              logical: 9,
              physical: "2026-05-18T14:25:00Z",
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
    });

    expect(document).toEqual({
      factoryDefinition: {
        name: "Current Factory",
        workers: [],
        workstations: [],
        workTypes: [],
      },
      version: {
        logical: 9,
        physical: "2026-05-18T14:25:00Z",
      },
    });
  });

  it("uses the session-scoped editable-definition route for non-default sessions", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factoryDefinition: {
            name: "Scoped Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
          version: {
            logical: 3,
            physical: "2026-05-18T14:24:00Z",
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

    await getCurrentEditableFactoryDefinitionDocument({
      fetch,
      sessionID: "session-2",
    });

    expect(fetch).toHaveBeenCalledWith(
      "/factories/session-2/factory/~current/editable-definition",
      {
        method: "GET",
      },
    );
  });

  it("surfaces current-factory transport failures with the original API error code", async () => {
    await expect(
      getCurrentEditableFactoryDefinition({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "NOT_FOUND",
              message: "Current factory definition not found.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 404,
              statusText: "Not Found",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "NOT_FOUND",
      message: "Current factory definition not found.",
      name: "CurrentEditableFactoryDefinitionError",
      status: 404,
      statusText: "Not Found",
    });
  });

  it("surfaces the existing unavailable-environment fallback when fetch is missing", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(
      getCurrentEditableFactoryDefinitionDocument(),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: "Current factory editing is unavailable in this environment.",
      name: "CurrentEditableFactoryDefinitionError",
    });
  });

  it("surfaces the existing network fallback when the editable-definition load request throws", async () => {
    await expect(
      getCurrentEditableFactoryDefinitionDocument({
        fetch: vi.fn().mockRejectedValue(new Error("socket closed")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message:
        "The dashboard could not reach the current factory editing API.",
      name: "CurrentEditableFactoryDefinitionError",
      responseBody: expect.any(Error),
    });
  });

  it("keeps the existing load rejection fallback when the API does not return a structured error message", async () => {
    await expect(
      getCurrentEditableFactoryDefinitionDocument({
        fetch: vi.fn().mockResolvedValue(
          new Response("", {
            status: 500,
            statusText: "Internal Server Error",
          }),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "The current factory editing API rejected the request.",
      name: "CurrentEditableFactoryDefinitionError",
      responseBody: null,
      status: 500,
      statusText: "Internal Server Error",
    });
  });

  it("normalizes INVALID_FACTORY editable-definition rejections into INVALID_FACTORY_DEFINITION", async () => {
    await expect(
      getCurrentEditableFactoryDefinitionDocument({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "INVALID_FACTORY",
              message: "The editable definition payload is invalid.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY_DEFINITION",
      message: "The editable definition payload is invalid.",
      name: "CurrentEditableFactoryDefinitionError",
      responseBody: {
        code: "INVALID_FACTORY",
        message: "The editable definition payload is invalid.",
      },
      status: 400,
      statusText: "Bad Request",
    });
  });

  it("rejects current-factory payloads that are not editable canonical factory definitions", async () => {
    let thrown: unknown;

    try {
      await getCurrentEditableFactoryDefinition({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              factoryDefinition: {
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
              },
              version: {
                logical: 12,
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
      });
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(CurrentEditableFactoryDefinitionError);
    expect(thrown).toMatchObject({
      code: "INVALID_FACTORY_DEFINITION",
      message:
        "The current factory editing API returned a factory definition the dashboard cannot edit. factory.workers[0].model must be a string.",
      responseBody: {
        factoryDefinition: {
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
        },
        version: {
          logical: 12,
          physical: "2026-05-18T14:30:00Z",
        },
      },
    });
  });

  it("saves the editable current-factory definition with version metadata", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
          version: {
            logical: 10,
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

    const document = await saveCurrentEditableFactoryDefinitionDocument(
      {
        baseVersion: {
          logical: 9,
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
      "/factories/~default/factory/~current/editable-definition",
      expect.objectContaining({
        body: JSON.stringify({
          baseVersion: {
            logical: 9,
            physical: "2026-05-18T14:25:00Z",
          },
          factoryDefinition: {
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      }),
    );
    expect(document.version.logical).toBe(10);
  });

  it("saves through the session-scoped editable-definition route for non-default sessions", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          factoryDefinition: {
            name: "Scoped Factory",
            workers: [],
            workstations: [],
            workTypes: [],
          },
          version: {
            logical: 11,
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

    await saveCurrentEditableFactoryDefinitionDocument(
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
      "/factories/session-2/factory/~current/editable-definition",
      expect.objectContaining({
        method: "PUT",
      }),
    );
  });

  it("preserves structured save error targets when the API rejects a topology edit", async () => {
    await expect(
      saveCurrentEditableFactoryDefinitionDocument(
        {
          baseVersion: {
            logical: 9,
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
                  {
                    id: "base-version",
                    kind: "save",
                  },
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
      targets: [
        {
          id: "base-version",
          kind: "save",
        },
      ],
    });
  });

  it("preserves valid structured save error targets from mixed target arrays", async () => {
    await expect(
      saveCurrentEditableFactoryDefinitionDocument(
        {
          baseVersion: {
            logical: 9,
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
                  {
                    id: "base-version",
                    kind: "save",
                  },
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
      targets: [
        {
          id: "base-version",
          kind: "save",
        },
      ],
    });
  });

  it("preserves active-work save rejections from the editable current-factory API", async () => {
    await expect(
      saveCurrentEditableFactoryDefinitionDocument(
        {
          baseVersion: {
            logical: 9,
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
                message: "Current factory runtime must be idle before activation.",
                targets: [
                  {
                    id: "active-work",
                    kind: "save",
                  },
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
        {
          id: "active-work",
          kind: "save",
        },
      ],
    });
  });

  it("surfaces the existing network fallback when the editable-definition save request throws", async () => {
    await expect(
      saveCurrentEditableFactoryDefinitionDocument(
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
      message:
        "The dashboard could not reach the current factory editing API.",
      name: "CurrentEditableFactoryDefinitionError",
      responseBody: expect.any(Error),
    });
  });

  it("keeps the existing save rejection fallback when the API does not return a structured error message", async () => {
    await expect(
      saveCurrentEditableFactoryDefinitionDocument(
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
      name: "CurrentEditableFactoryDefinitionError",
      responseBody: null,
      status: 500,
      statusText: "Internal Server Error",
    });
  });
});
