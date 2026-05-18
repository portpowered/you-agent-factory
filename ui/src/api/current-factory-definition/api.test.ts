import {
  CurrentEditableFactoryDefinitionError,
  getCurrentEditableFactoryDefinition,
} from "./api";

describe("getCurrentEditableFactoryDefinition", () => {
  it("loads the current factory through the existing API and preserves editable workstation fields", async () => {
    const factoryDefinition = await getCurrentEditableFactoryDefinition({
      fetch: vi.fn().mockResolvedValue(
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
        "The current factory API returned a factory definition the dashboard cannot edit. factory.workers[0].model must be a string.",
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
      },
    });
  });
});
