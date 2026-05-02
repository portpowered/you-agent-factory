import {
  FactoryDefinitionAPIError,
  isCanonicalFactoryDefinition,
  normalizeFactoryDefinition,
} from "./api";

describe("normalizeFactoryDefinition", () => {
  it("accepts canonical generated factory payloads", () => {
    expect(
      normalizeFactoryDefinition({
        guards: [
          {
            modelProvider: "CLAUDE",
            model: "claude-sonnet-4-20250514",
            refreshWindow: "15m",
            type: "INFERENCE_THROTTLE_GUARD",
          },
        ],
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
      }),
    ).toEqual({
      name: "agent-factory",
      guards: [
        {
          modelProvider: "CLAUDE",
          model: "claude-sonnet-4-20250514",
          refreshWindow: "15m",
          type: "INFERENCE_THROTTLE_GUARD",
        },
      ],
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

  it("rejects malformed factory-level throttle guards", () => {
    expect(() =>
      normalizeFactoryDefinition({
        guards: [
          {
            modelProvider: "claude",
            refreshWindow: "15m",
            type: "INFERENCE_THROTTLE_GUARD",
          },
        ],
        name: "legacy-factory",
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.guards[0].modelProvider must be one of CLAUDE, CODEX.",
      ),
    );
  });

  it("rejects retired legacy field aliases in the UI boundary", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "legacy-factory",
        workers: [
          {
            model_provider: "CODEX",
            name: "writer",
          },
        ],
        workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
        workstations: [
          {
            definition: {
              runtime_type: "MODEL_WORKSTATION",
            },
            inputs: [{ state: "new", work_type: "story" }],
            name: "Draft",
            on_failure: { state: "failed", work_type: "story" },
            outputs: [{ state: "done", work_type: "story" }],
            resource_usage: [{ capacity: 1, name: "slot" }],
            stop_token: "DONE",
            worker: "writer",
          },
        ],
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.workers[0].model_provider is not allowed by the generated factory contract.",
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
