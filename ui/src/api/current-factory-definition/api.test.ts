import {
  CurrentEditableFactoryDefinitionError,
  getCurrentEditableFactoryDefinition,
  getCurrentEditableFactoryDefinitionDocument,
  saveCurrentEditableFactoryDefinitionDocument,
} from "./api";

describe("getCurrentEditableFactoryDefinition", () => {
  it("loads the current factory through the existing API and preserves editable workstation fields", async () => {
    const factoryDefinition = await getCurrentEditableFactoryDefinition({
      fetch: vi.fn().mockResolvedValue(
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
      ),
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
      expect.stringContaining("/factory/~current/editable-definition"),
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
});
