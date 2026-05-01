import {
  FactoryDefinitionAPIError,
  isCanonicalFactoryDefinition,
  normalizeFactoryDefinition,
} from "./api";

describe("normalizeFactoryDefinition", () => {
  it("normalizes supported public shape aliases into the generated factory contract", () => {
    expect(
      normalizeFactoryDefinition({
        input_types: [{ name: "default", type: "DEFAULT" }],
        id: "agent-factory",
        name: "agent-factory",
        source_directory: "/tmp/legacy-factory",
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
                max_visits: 3,
                type: "VISIT_COUNT",
                workstation: "Draft",
              },
            ],
            inputs: [
              {
                guards: [{ match_input: "planItem", type: "SAME_NAME" }],
                state: "new",
                work_type: "story",
              },
            ],
            kind: "STANDARD",
            name: "Draft",
            on_failure: { state: "failed", work_type: "story" },
            outputs: [{ state: "done", work_type: "story" }],
            worker: "writer",
          },
        ],
      }),
    ).toEqual({
      name: "agent-factory",
      inputTypes: [{ name: "default", type: "DEFAULT" }],
      id: "agent-factory",
      sourceDirectory: "/tmp/legacy-factory",
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
          kind: "STANDARD",
          name: "Draft",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "done", workType: "story" }],
          worker: "writer",
        },
      ],
    });
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
