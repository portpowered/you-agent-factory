import {
  FactoryDefinitionAPIError,
  isCanonicalFactoryDefinition,
  normalizeFactoryDefinition,
} from "./api";

describe("normalizeFactoryDefinition", () => {
  it("accepts canonical generated factory payloads", () => {
    expect(
      normalizeFactoryDefinition({
        inputTypes: [{ name: "default", type: "DEFAULT" }],
        id: "agent-factory",
        name: "agent-factory",
        sourceDirectory: "/tmp/canonical-factory",
        supportingFiles: {
          requiredTools: [{ command: "python", name: "python" }],
        },
        workers: [
          {
            modelProvider: "CODEX",
            name: "writer",
            session_id: "sess-123",
            type: "MODEL_WORKER",
          },
        ],
        work_types: [
          {
            name: "story",
            states: [{ name: "new", type: "INITIAL" }],
          },
        ],
        workstations: [
          {
            guards: [
              {
                maxVisits: 3,
                type: "VISIT_COUNT",
                workstation: "Draft",
              },
            ],
            inputs: [
              {
                guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
                state: "new",
                workType: "story",
              },
            ],
            behavior: "STANDARD",
            name: "Draft",
            onFailure: { state: "failed", workType: "story" },
            outputs: [{ state: "done", workType: "story" }],
            worker: "writer",
          },
        ],
      }),
    ).toEqual({
      name: "agent-factory",
      inputTypes: [{ name: "default", type: "DEFAULT" }],
      id: "agent-factory",
      sourceDirectory: "/tmp/canonical-factory",
      supportingFiles: {
        requiredTools: [{ command: "python", name: "python" }],
      },
      workers: [
        {
          modelProvider: "CODEX",
          name: "writer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [{ name: "new", type: "INITIAL" }],
        },
      ],
      workstations: [
        {
          guards: [
            {
              maxVisits: 3,
              type: "VISIT_COUNT",
              workstation: "Draft",
            },
          ],
          inputs: [
            {
              guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
              state: "new",
              workType: "story",
            },
          ],
          behavior: "STANDARD",
          name: "Draft",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "done", workType: "story" }],
          worker: "writer",
        },
      ],
    });
  });

  it("rejects retired lowercase public enum aliases", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "legacy-factory",
        workstations: [
          {
            behavior: "repeater",
            inputs: [{ state: "new", workType: "story" }],
            name: "Draft",
            outputs: [{ state: "done", workType: "story" }],
            worker: "writer",
          },
        ],
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.workstations[0].behavior must be one of CRON, REPEATER, STANDARD.",
      ),
    );
  });

  it("rejects fields outside the generated contract", () => {
    expect(() =>
      normalizeFactoryDefinition({
        project: "legacy-factory",
        name: "legacy-factory",
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.project is not allowed by the generated factory contract.",
      ),
    );
  });
});

describe("isCanonicalFactoryDefinition", () => {
  it("returns true for canonical generated payloads", () => {
    expect(
      isCanonicalFactoryDefinition({
        id: "agent-factory",
        name: "agent-factory",
        workstations: [
          {
            inputs: [{ state: "new", workType: "story" }],
            name: "Draft",
            outputs: [{ state: "done", workType: "story" }],
            worker: "writer",
          },
        ],
      }),
    ).toBe(true);
  });

  it("returns false for payloads outside the contract", () => {
    expect(
      isCanonicalFactoryDefinition({
        workstations: [{ name: "Draft" }],
      }),
    ).toBe(false);
  });
});
