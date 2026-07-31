import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import {
  applyFactoryGraphAddEntityDraft,
  createFactoryGraphAddEntityDraft,
  validateFactoryGraphAddEntityDraft,
} from "../editor/factory-graph-editor-additions";

const minimalModelWorkerOperation = {
  name: "REVIEW",
  inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
  outputs: [{ name: "result", contentTypes: ["TEXT"] }],
};

const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
      ],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "draft",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph add validation and apply scenarios share one fixture factory.
describe("factory graph editor additions", () => {
  it("seeds workstation and work-state drafts from the current pending definition", () => {
    expect(
      createFactoryGraphAddEntityDraft("workstation", baseFactoryDefinition),
    ).toMatchObject({
      behavior: "STANDARD",
      cron: null,
      kind: "workstation",
      workerName: "writer",
      workstationType: "INFERENCE_RUN",
    });
    expect(
      createFactoryGraphAddEntityDraft("work-state", baseFactoryDefinition),
    ).toMatchObject({
      kind: "work-state",
      stateType: "PROCESSING",
      workTypeName: "story",
    });
    expect(
      createFactoryGraphAddEntityDraft("worker", baseFactoryDefinition),
    ).toEqual({
      argsText: "",
      command: "",
      kind: "worker",
      model: "",
      modelProvider: "",
      name: "",
      operations: [],
      provider: "",
      workerType: "INFERENCE_WORKER",
    });
  });

  it("allows provider-only worker adds without operations and rejects missing model provider", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "CODEX",
          name: "reviewer",
          operations: [],
          provider: "",
          workerType: "INFERENCE_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({});

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "reviewer",
          operations: [],
          provider: "",
          workerType: "INFERENCE_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      modelProvider: "Select a model provider for the new worker.",
    });
  });

  it("requires script worker command and rejects null bytes in args text", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "node",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "runner",
          operations: [],
          provider: "",
          workerType: "SCRIPT_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({});

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "runner",
          operations: [],
          provider: "",
          workerType: "SCRIPT_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      command: "Enter a command for the new script worker.",
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "check\0lint",
          command: "node",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "runner",
          operations: [],
          provider: "",
          workerType: "SCRIPT_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      args: "Each script argument must be a single non-empty line.",
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "node",
          kind: "worker",
          model: "",
          modelProvider: "CODEX",
          name: "runner",
          operations: [],
          provider: "",
          workerType: "SCRIPT_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({});
  });

  it("rejects duplicate identifiers and structurally invalid add forms", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "CODEX",
          name: "writer",
          operations: [minimalModelWorkerOperation],
          provider: "",
          workerType: "INFERENCE_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      name: 'A worker named "writer" already exists in the draft.',
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          behavior: "POLLER",
          body: "",
          cron: null,
          kind: "workstation",
          name: "linear-poller",
          workerName: "writer",
          workstationType: "AGENT_RUN",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      behavior: "Poller workstations must use a script or hosted worker.",
    });

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          behavior: "POLLER",
          body: "",
          cron: null,
          kind: "workstation",
          name: "linear-poller",
          workerName: "poller-runner",
          workstationType: "AGENT_RUN",
        },
        {
          ...baseFactoryDefinition,
          workers: [
            ...(baseFactoryDefinition.workers ?? []),
            {
              command: "node",
              name: "poller-runner",
              type: "SCRIPT_WORKER",
            },
          ],
        },
      ),
    ).toEqual({});

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          capacity: "0",
          kind: "resource",
          name: "gpu",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      capacity: "Resource capacity must be a whole number greater than zero.",
    });
  });

  it("appends poller behavior to new workstations in canonical factory shape", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "POLLER",
        body: "Review the story output.",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
    );

    expect(nextDraft.additions.workstations).toEqual([
      {
        behavior: "POLLER",
        body: "Review the story output.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "AGENT_RUN",
        worker: "writer",
      },
    ]);
  });

  it("persists modelProvider on new workers and model only when non-empty", () => {
    const providerOnlyDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "CODEX",
        name: "reviewer",
        operations: [],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
    );
    expect(providerOnlyDraft.additions.workers).toEqual([
      {
        modelProvider: "CODEX",
        name: "reviewer",
        type: "INFERENCE_WORKER",
      },
    ]);

    const withModelDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "",
        kind: "worker",
        model: "gpt-5",
        modelProvider: "CODEX",
        name: "writer",
        operations: [],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
    );
    expect(withModelDraft.additions.workers).toEqual([
      {
        model: "gpt-5",
        modelProvider: "CODEX",
        name: "writer",
        type: "INFERENCE_WORKER",
      },
    ]);
  });

  it("persists script workers with command and parsed args", () => {
    const commandOnlyDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "node",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "runner",
        operations: [],
        provider: "",
        workerType: "SCRIPT_WORKER",
      },
    );
    expect(commandOnlyDraft.additions.workers).toEqual([
      {
        command: "node",
        name: "runner",
        type: "SCRIPT_WORKER",
      },
    ]);

    const withArgsDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: " --verbose \n\n--dry-run\n",
        command: "npm",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "packager",
        operations: [],
        provider: "",
        workerType: "SCRIPT_WORKER",
      },
    );
    expect(withArgsDraft.additions.workers).toEqual([
      {
        args: ["--verbose", "--dry-run"],
        command: "npm",
        name: "packager",
        type: "SCRIPT_WORKER",
      },
    ]);
  });

  it("omits explicit standard behavior for new workstations", () => {
    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        behavior: "STANDARD",
        body: "Review the story output.",
        cron: null,
        kind: "workstation",
        name: "review",
        workerName: "writer",
        workstationType: "AGENT_RUN",
      },
    );

    expect(nextDraft.additions.workstations).toEqual([
      {
        body: "Review the story output.",
        inputs: [],
        name: "review",
        outputs: [],
        type: "AGENT_RUN",
        worker: "writer",
      },
    ]);
  });
});
