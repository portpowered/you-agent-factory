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
            onContinue: [
              { state: "new", workType: "story" },
              { state: "queued", workType: "story" },
            ],
            onFailure: [
              { state: "failed", workType: "story" },
              { state: "blocked", workType: "story" },
            ],
            onRejection: [
              { state: "needs-review", workType: "story" },
              { state: "rejected", workType: "story" },
            ],
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
          onContinue: [
            { state: "new", workType: "story" },
            { state: "queued", workType: "story" },
          ],
          onFailure: [
            { state: "failed", workType: "story" },
            { state: "blocked", workType: "story" },
          ],
          onRejection: [
            { state: "needs-review", workType: "story" },
            { state: "rejected", workType: "story" },
          ],
          outputs: [{ state: "done", workType: "story" }],
          worker: "writer",
        },
      ],
    });
  });

  it("preserves fully populated canonical worker, guard, and workstation options", () => {
    expect(
      normalizeFactoryDefinition({
        factoryDirectory: "/tmp/factory",
        guards: [
          {
            model: "claude-sonnet-4-20250514",
            modelProvider: "CLAUDE",
            refreshWindow: "15m",
            type: "INFERENCE_THROTTLE_GUARD",
          },
        ],
        id: "agent-factory",
        inputTypes: [{ name: "default", type: "DEFAULT" }],
        metadata: {
          owner: "frontend-tests",
        },
        name: "agent-factory",
        resources: [{ capacity: 3, name: "gpu" }],
        sourceDirectory: "/tmp/source-factory",
        supportingFiles: {
          scripts: {
            draft: "draft.sh",
          },
        },
        workers: [
          {
            args: ["--json"],
            body: "echo ready",
            command: "runner",
            executorProvider: "SCRIPT_WRAP",
            model: "codex-mini",
            modelProvider: "CODEX",
            name: "writer",
            resources: [{ capacity: 1, name: "gpu" }],
            skipPermissions: true,
            stopToken: "DONE",
            timeout: "15m",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "running", type: "PROCESSING" },
              { name: "done", type: "TERMINAL" },
              { name: "failed", type: "FAILED" },
            ],
          },
        ],
        workstations: [
          {
            behavior: "CRON",
            body: "plan.sh",
            copyReferencedScripts: true,
            cron: {
              expiryWindow: "5m",
              jitter: "30s",
              schedule: "*/5 * * * *",
              triggerAtStart: true,
            },
            env: {
              MODE: "test",
            },
            guards: [
              {
                matchConfig: { inputKey: "storyId" },
                maxVisits: 3,
                type: "MATCHES_FIELDS",
                workstation: "Review",
              },
            ],
            id: "draft-station",
            inputs: [
              {
                guards: [
                  { matchInput: "storyId", type: "SAME_NAME" },
                  { parentInput: "storyId", type: "ALL_CHILDREN_COMPLETE" },
                  { spawnedBy: "draft", type: "ANY_CHILD_FAILED" },
                ],
                state: "new",
                workType: "story",
              },
            ],
            limits: {
              maxExecutionTime: "30m",
              maxRetries: 2,
            },
            name: "Draft",
            onContinue: [{ state: "running", workType: "story" }],
            onFailure: [{ state: "failed", workType: "story" }],
            onRejection: [{ state: "new", workType: "story" }],
            outputSchema: "schema.json",
            outputs: [{ state: "done", workType: "story" }],
            promptFile: "prompt.md",
            resources: [{ capacity: 1, name: "gpu" }],
            stopWords: ["DONE"],
            type: "MODEL_WORKSTATION",
            worker: "writer",
            workingDirectory: "/tmp/workdir",
            worktree: "detached",
          },
        ],
      }),
    ).toEqual({
      factoryDirectory: "/tmp/factory",
      guards: [
        {
          model: "claude-sonnet-4-20250514",
          modelProvider: "CLAUDE",
          refreshWindow: "15m",
          type: "INFERENCE_THROTTLE_GUARD",
        },
      ],
      id: "agent-factory",
      inputTypes: [{ name: "default", type: "DEFAULT" }],
      metadata: {
        owner: "frontend-tests",
      },
      name: "agent-factory",
      resources: [{ capacity: 3, name: "gpu" }],
      sourceDirectory: "/tmp/source-factory",
      supportingFiles: {
        scripts: {
          draft: "draft.sh",
        },
      },
      workers: [
        {
          args: ["--json"],
          body: "echo ready",
          command: "runner",
          executorProvider: "SCRIPT_WRAP",
          model: "codex-mini",
          modelProvider: "CODEX",
          name: "writer",
          resources: [{ capacity: 1, name: "gpu" }],
          skipPermissions: true,
          stopToken: "DONE",
          timeout: "15m",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "running", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          behavior: "CRON",
          body: "plan.sh",
          copyReferencedScripts: true,
          cron: {
            expiryWindow: "5m",
            jitter: "30s",
            schedule: "*/5 * * * *",
            triggerAtStart: true,
          },
          env: {
            MODE: "test",
          },
          guards: [
            {
              matchConfig: { inputKey: "storyId" },
              maxVisits: 3,
              type: "MATCHES_FIELDS",
              workstation: "Review",
            },
          ],
          id: "draft-station",
          inputs: [
            {
              guards: [
                { matchInput: "storyId", type: "SAME_NAME" },
                { parentInput: "storyId", type: "ALL_CHILDREN_COMPLETE" },
                { spawnedBy: "draft", type: "ANY_CHILD_FAILED" },
              ],
              state: "new",
              workType: "story",
            },
          ],
          limits: {
            maxExecutionTime: "30m",
            maxRetries: 2,
          },
          name: "Draft",
          onContinue: [{ state: "running", workType: "story" }],
          onFailure: [{ state: "failed", workType: "story" }],
          onRejection: [{ state: "new", workType: "story" }],
          outputSchema: "schema.json",
          outputs: [{ state: "done", workType: "story" }],
          promptFile: "prompt.md",
          resources: [{ capacity: 1, name: "gpu" }],
          stopWords: ["DONE"],
          type: "MODEL_WORKSTATION",
          worker: "writer",
          workingDirectory: "/tmp/workdir",
          worktree: "detached",
        },
      ],
    });
  });

  it("defaults cron triggerAtStart to false when it is omitted", () => {
    expect(
      normalizeFactoryDefinition({
        name: "agent-factory",
        workstations: [
          {
            cron: {
              schedule: "0 * * * *",
            },
            inputs: [{ state: "new", workType: "story" }],
            name: "Draft",
            outputs: [{ state: "done", workType: "story" }],
            worker: "writer",
          },
        ],
      }),
    ).toEqual({
      name: "agent-factory",
      workstations: [
        {
          cron: {
            schedule: "0 * * * *",
            triggerAtStart: false,
          },
          inputs: [{ state: "new", workType: "story" }],
          name: "Draft",
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

  it("rejects unsupported factory-level guard types", () => {
    expect(() =>
      normalizeFactoryDefinition({
        guards: [
          {
            modelProvider: "CLAUDE",
            refreshWindow: "15m",
            type: "VISIT_COUNT",
          },
        ],
        name: "legacy-factory",
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.guards[0].type must be one of INFERENCE_THROTTLE_GUARD.",
      ),
    );
  });

  it("rejects inference throttle guards on workstations", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "legacy-factory",
        workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
        workers: [{ name: "writer" }],
        workstations: [
          {
            name: "Draft",
            worker: "writer",
            guards: [{ type: "INFERENCE_THROTTLE_GUARD" }],
            inputs: [{ state: "new", workType: "story" }],
            outputs: [{ state: "new", workType: "story" }],
          },
        ],
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.workstations[0].guards[0].type must be one of VISIT_COUNT, MATCHES_FIELDS.",
      ),
    );
  });

  it("rejects inference throttle guards on inputs", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "legacy-factory",
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workers: [{ name: "writer" }],
        workstations: [
          {
            name: "Draft",
            worker: "writer",
            inputs: [
              {
                state: "new",
                workType: "story",
                guards: [{ type: "INFERENCE_THROTTLE_GUARD" }],
              },
            ],
            outputs: [{ state: "done", workType: "story" }],
          },
        ],
      }),
    ).toThrowError(
      new FactoryDefinitionAPIError(
        "factory.workstations[0].inputs[0].guards[0].type must be one of VISIT_COUNT, ALL_CHILDREN_COMPLETE, ANY_CHILD_FAILED, SAME_NAME.",
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
