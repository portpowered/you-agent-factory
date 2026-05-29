// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: existing workstation-editable-values coverage stayed intact during feature-root migration.
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  applyEditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "./workstation-editable-values";

const selectedNode: DashboardWorkstationNode = {
  model: "gpt-5.4",
  node_id: "review",
  transition_id: "review",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Review",
};

describe("resolveEditableWorkstationValues", () => {
  it("joins the selected workstation with the canonical worker options", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the latest story changes before approval.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Review",
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("defaults omitted workstation type to MODEL_WORKSTATION", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.workstationType,
    ).toBe("MODEL_WORKSTATION");
  });

  it("preserves explicit workstation implementation types", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Move work downstream.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          type: "LOGICAL_MOVE",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.workstationType,
    ).toBe("LOGICAL_MOVE");
  });

  it("falls back from transition id lookup to workstation name lookup", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.workstationName,
    ).toBe("Review");
  });

  it("keeps the selected workstation visible when its current worker is missing from the canonical worker list", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "missing-worker",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the latest story changes before approval.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "missing-worker",
      workerOptions: [],
      workerTypeByName: {},
      workstationName: "Review",
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("applies editable draft changes without rewriting unsupported workstation or worker fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      metadata: {
        description: "Factory metadata must survive workstation edits.",
      },
      resources: [
        {
          initial: 2,
          name: "review-slots",
        },
      ],
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          limits: { maxRetries: 3 },
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          stopWords: ["STOP"],
          worker: "reviewer",
          workingDirectory: "/repo/review",
        },
      ],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        behavior: "POLLER",
        prompt: "Review the updated prompt before approval.",
        runnerName: null,
        workerName: "reviewer",
      },
    );

    expect(updatedFactory).toMatchObject({
      metadata: {
        description: "Factory metadata must survive workstation edits.",
      },
      resources: [
        {
          initial: 2,
          name: "review-slots",
        },
      ],
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          name: "reviewer",
        },
      ],
      workstations: [
        {
          behavior: "POLLER",
          body: "Review the updated prompt before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          limits: { maxRetries: 3 },
          promptFile: "prompts/review.md",
          stopWords: ["STOP"],
          workingDirectory: "/repo/review",
        },
      ],
      workTypes: [
        {
          name: "story",
        },
      ],
    });
    expect(updatedFactory?.workers).toBe(factory.workers);
    expect(updatedFactory?.workTypes).toBe(factory.workTypes);
    expect(updatedFactory?.resources).toBe(factory.resources);
  });

  it("returns every current worker as a worker option", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Code work",
          id: "code",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Code",
          outputs: [{ state: "implemented", workType: "story" }],
          worker: "coder",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review work",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {
        coder: ["Code"],
        processor: ["Plan"],
      },
      sharedWorkerWorkstationNames: ["Plan"],
      workerModelProvider: null,
      workerName: "processor",
      workerOptions: ["processor"],
      workerTypeByName: {
        processor: "MODEL_WORKER",
      },
      workstationName: "Review",
      workstationType: "MODEL_WORKSTATION",
    });
  });

  it("resolves legacy_provider runner selection from worker modelProvider", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toMatchObject({
      effectiveRunnerName: "codex",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "legacy_provider",
      },
      runnerSelectionSource: "legacy_provider",
      workerModelProvider: "CODEX",
    });
  });

  it("applies worker switches through the workstation field without rewriting worker models", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Code work",
          id: "code",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Code",
          outputs: [{ state: "implemented", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        behavior: "STANDARD",
        prompt: "Updated review work",
        runnerName: "gemini",
        workerName: "reviewer",
      }),
    ).toMatchObject({
      workers: [
        { model: "gpt-5.4", name: "processor" },
        { model: "gpt-5.5", name: "reviewer" },
      ],
      workstations: [
        {
          body: "Updated review work",
          name: "Review",
          runner: "gemini",
          worker: "reviewer",
        },
        { body: "Plan work", name: "Plan" },
        { body: "Code work", name: "Code" },
      ],
    });
  });

  it("keeps shared worker objects unchanged when applying prompt and behavior changes", () => {
    const sharedWorker = {
      body: "Shared worker instructions stay worker-owned.",
      model: "gpt-5.4",
      name: "processor",
      type: "MODEL_WORKER" as const,
    };
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [sharedWorker],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        behavior: "REPEATER",
        prompt: "Updated review prompt only.",
        runnerName: null,
        workerName: "processor",
      },
    );

    expect(updatedFactory?.workers).toBe(factory.workers);
    expect(updatedFactory?.workers?.[0]).toBe(sharedWorker);
    expect(updatedFactory).toMatchObject({
      workers: [
        {
          body: "Shared worker instructions stay worker-owned.",
          model: "gpt-5.4",
          name: "processor",
        },
      ],
      workstations: [
        {
          behavior: "REPEATER",
          body: "Updated review prompt only.",
          name: "Review",
          worker: "processor",
        },
        {
          body: "Plan work",
          name: "Plan",
          worker: "processor",
        },
      ],
    });
  });

  it("rejects worker switches when the selected worker is no longer available", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        behavior: "STANDARD",
        prompt: "Updated review work",
        runnerName: null,
        workerName: "missing-worker",
      }),
    ).toBeNull();
  });

  it("keeps cron behavior selectable for existing cron workstations", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          command: "./cron.sh",
          name: "cron-runner",
          type: "SCRIPT_WORKER",
        },
      ],
      workstations: [
        {
          behavior: "CRON",
          cron: { schedule: "0 * * * *" },
          inputs: [],
          name: "Review",
          outputs: [],
          worker: "cron-runner",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toMatchObject({
      behavior: "CRON",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
    });
  });
});
