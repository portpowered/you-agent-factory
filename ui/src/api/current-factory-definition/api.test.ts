import {
  factoryRuntimeNotIdleTarget,
  staleFactoryVersionTarget,
} from "../../testing/factory-validation-target-fixtures";
import {
  CurrentFactoryDefinitionError,
  getCurrentFactoryDefinition,
  getCurrentFactoryDocument,
  saveCurrentFactoryDocument,
} from "./api";

describe("getCurrentFactoryDefinition", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads the current factory through the existing API and preserves editable workstation fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
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
          version: {
            logical: "7",
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

    const factoryDefinition = await getCurrentFactoryDefinition({
      fetch: fetchMock,
    });

    expect(factoryDefinition).toEqual({
      id: "factory-current",
      name: "Current Factory",
      version: {
        logical: "7",
        physical: "2026-05-18T14:22:00Z",
      },
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
      "/factory-sessions/~default/factory",
      {
        method: "GET",
      },
    );
  });

  it("returns version metadata together with the editable current factory definition document", async () => {
    const document = await getCurrentFactoryDocument({
      fetch: vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            name: "Current Factory",
            workers: [],
            workstations: [],
            workTypes: [],
            version: {
              logical: "9",
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
      name: "Current Factory",
      workers: [],
      workstations: [],
      workTypes: [],
      version: {
        logical: "9",
        physical: "2026-05-18T14:25:00Z",
      },
    });
  });

  it("uses the session-scoped editable-definition route for non-default sessions", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Scoped Factory",
          workers: [],
          workstations: [],
          workTypes: [],
          version: {
            logical: "3",
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

    await getCurrentFactoryDocument({
      fetch,
      sessionID: "session-2",
    });

    expect(fetch).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      {
        method: "GET",
      },
    );
  });

  it("surfaces current-factory transport failures with the original API error code", async () => {
    await expect(
      getCurrentFactoryDefinition({
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
      name: "CurrentFactoryDefinitionError",
      status: 404,
      statusText: "Not Found",
    });
  });

  it("surfaces the existing unavailable-environment fallback when fetch is missing", async () => {
    vi.stubGlobal("fetch", undefined);

    await expect(
      getCurrentFactoryDocument(),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message: "Current factory editing is unavailable in this environment.",
      name: "CurrentFactoryDefinitionError",
    });
  });

  it("surfaces the existing network fallback when the editable-definition load request throws", async () => {
    await expect(
      getCurrentFactoryDocument({
        fetch: vi.fn().mockRejectedValue(new Error("socket closed")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      message:
        "The dashboard could not reach the current factory editing API.",
      name: "CurrentFactoryDefinitionError",
      responseBody: expect.any(Error),
    });
  });

  it("keeps the existing load rejection fallback when the API does not return a structured error message", async () => {
    await expect(
      getCurrentFactoryDocument({
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
      name: "CurrentFactoryDefinitionError",
      responseBody: null,
      status: 500,
      statusText: "Internal Server Error",
    });
  });

  it("normalizes INVALID_FACTORY editable-definition rejections into INVALID_FACTORY_DEFINITION", async () => {
    await expect(
      getCurrentFactoryDocument({
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
      message: "The factory definition was rejected by the session factory API.",
      name: "CurrentFactoryDefinitionError",
      responseBody: {
        code: "INVALID_FACTORY",
        message: "The editable definition payload is invalid.",
      },
      status: 400,
      statusText: "Bad Request",
    });
  });

  it("rejects load success payloads that are not editable-definition documents", async () => {
    await expect(
      getCurrentFactoryDocument({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
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
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "The current factory editing API returned an invalid response.",
      name: "CurrentFactoryDefinitionError",
      responseBody: {
        version: {
          logical: "12",
          physical: "2026-05-18T14:30:00Z",
        },
      },
      status: 200,
      statusText: "OK",
    });
  });

  it("rejects current-factory payloads that are not editable canonical factory definitions", async () => {
    let thrown: unknown;

    try {
      await getCurrentFactoryDefinition({
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
      });
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(CurrentFactoryDefinitionError);
    expect(thrown).toMatchObject({
      code: "INVALID_FACTORY_DEFINITION",
      message:
        "The current factory editing API returned a factory definition the dashboard cannot edit. factory.workers[0].model must be a string.",
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
    });
  });

  it("accepts editable-definition payloads with classifier workstations", async () => {
    const document = await getCurrentFactoryDocument({
      fetch: vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            name: "Current Factory",
            workers: [{ name: "classifier" }],
            workTypes: [
              {
                name: "story",
                states: [
                  { name: "new", type: "INITIAL" },
                  { name: "approved", type: "TERMINAL" },
                  { name: "failed", type: "FAILED" },
                ],
              },
            ],
            workstations: [
              {
                classificationRoutes: [
                  {
                    label: "approved",
                    outputs: [{ state: "approved", workType: "story" }],
                  },
                ],
                inputs: [{ state: "new", workType: "story" }],
                name: "Classify",
                onFailure: [{ state: "failed", workType: "story" }],
                type: "CLASSIFIER_WORKSTATION",
                worker: "classifier",
              },
            ],
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
    });

    expect(document).toEqual({
      name: "Current Factory",
      workers: [{ name: "classifier" }],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "approved", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          classificationRoutes: [
            {
              label: "approved",
              outputs: [{ state: "approved", workType: "story" }],
            },
          ],
          inputs: [{ state: "new", workType: "story" }],
          name: "Classify",
          onFailure: [{ state: "failed", workType: "story" }],
          type: "CLASSIFIER_WORKSTATION",
          worker: "classifier",
        },
      ],
      version: {
        logical: "12",
        physical: "2026-05-18T14:30:00Z",
      },
    });
  });

  it("rejects editable-definition payloads with unexpected classifier-route keys", async () => {
    await expect(
      getCurrentFactoryDocument({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Current Factory",
              workers: [{ name: "classifier" }],
              workTypes: [
                {
                  name: "story",
                  states: [
                    { name: "new", type: "INITIAL" },
                    { name: "approved", type: "TERMINAL" },
                    { name: "failed", type: "FAILED" },
                  ],
                },
              ],
              workstations: [
                {
                  classificationRoutes: [
                    {
                      label: "approved",
                      outputs: [{ state: "approved", workType: "story" }],
                      unexpected: "x",
                    },
                  ],
                  inputs: [{ state: "new", workType: "story" }],
                  name: "Classify",
                  onFailure: [{ state: "failed", workType: "story" }],
                  type: "CLASSIFIER_WORKSTATION",
                  worker: "classifier",
                },
              ],
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
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY_DEFINITION",
      message:
        "The current factory editing API returned a factory definition the dashboard cannot edit. factory.workstations[0].classificationRoutes[0].unexpected is not allowed by the generated factory contract.",
      name: "CurrentFactoryDefinitionError",
      status: 200,
      statusText: "OK",
    });
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
        body: expect.stringContaining(
          '"factory":{"name":"Current Factory"',
        ),
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
        body: expect.stringContaining(
          '"physical":"2026-05-28T04:11:21.570Z"',
        ),
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
        vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === "PUT")?.[1]
          ?.body,
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
                message: "Current factory runtime must be idle before activation.",
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
      message:
        "The dashboard could not reach the current factory editing API.",
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
});
