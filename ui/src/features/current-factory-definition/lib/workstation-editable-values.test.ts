// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/style/noExcessiveLinesPerFile: existing workstation-editable-values coverage stayed intact during feature-root migration.
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import { WorkerType, WorkstationType } from "../../../api/generated/openapi";
import { resolveEditableWorkstationOverwriteFields } from "../../current-selection/workstation-selection/editing/editable-workstation-overwrite-fields";
import {
  applyEditableWorkstationDraft,
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationCron,
  resolveEditableWorkstationValues,
} from "./workstation-editable-values";
import { editableWorkstationDraftsEqual } from "./workstation-guards";

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
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {
        reviewer: [],
      },
      operation: "",
      operationBindings: [],
      prompt: "Review the latest story changes before approval.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode", "pi"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      guards: [],
      inputs: [{ guards: [], state: "queued", workType: "story" }],
      workstationName: "Review",
      workstationOptions: ["Review"],
      workstationType: "AGENT_RUN",
      workstationTypeOptions: ["AGENT_RUN", "INFERENCE_RUN"],
    });
  });

  it("resolves model invoke workstation operation and bindings from the factory", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "tts-factory",
      workers: [
        {
          name: "tts-worker",
          type: "MODEL_WORKER",
          operations: [
            {
              name: "TTS",
              inputs: [
                { name: "text", contentTypes: ["TEXT"], required: true },
              ],
              outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
            },
          ],
        },
      ],
      workstations: [
        {
          name: "speak-story",
          type: "INFERENCE_RUN",
          worker: "tts-worker",
          operation: "TTS",
          operationBindings: [
            {
              slot: "text",
              selector: { label: "utterance", type: "TEXT" },
            },
          ],
          inputs: [{ state: "init", workType: "story" }],
          outputs: [{ state: "complete", workType: "story" }],
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, {
        ...selectedNode,
        transition_id: "speak-story",
        workstation_name: "speak-story",
      }),
    ).toMatchObject({
      modelInvokeWorkerOptions: ["tts-worker"],
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          configText: "",
          defaultContentText: "",
          selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
        },
      ],
      workstationType: "INFERENCE_RUN",
    });
  });

  it("applies model invoke workstation edits without mutating unrelated factory data", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "tts-factory",
      workers: [
        {
          name: "tts-worker",
          type: "MODEL_WORKER",
          operations: [
            {
              name: "TTS",
              inputs: [
                { name: "text", contentTypes: ["TEXT"], required: true },
                { name: "voice", contentTypes: ["JSON"] },
              ],
              outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
            },
          ],
        },
        {
          name: "reviewer",
          type: "MODEL_WORKER",
          model: "gpt-5.4",
        },
      ],
      workstations: [
        {
          name: "speak-story",
          type: "INFERENCE_RUN",
          worker: "tts-worker",
          operation: "TTS",
          operationBindings: [
            {
              slot: "text",
              selector: { label: "utterance", type: "TEXT" },
            },
          ],
          inputs: [{ state: "init", workType: "story" }],
          outputs: [{ state: "complete", workType: "story" }],
        },
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      {
        ...selectedNode,
        transition_id: "speak-story",
        workstation_name: "speak-story",
      },
      {
        behavior: "STANDARD",
        cron: null,
        guards: [],
        inputs: [{ guards: [], state: "init", workType: "story" }],
        name: "speak-story",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "static narration",
            defaultContentText: "fallback narration",
            selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
          },
          {
            slot: "voice",
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        prompt: "",
        runnerName: null,
        workerName: "tts-worker",
        workstationType: "INFERENCE_RUN",
      },
    );

    expect(updatedFactory?.workstations?.[0]).toEqual({
      name: "speak-story",
      type: "INFERENCE_RUN",
      worker: "tts-worker",
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          selector: { label: "utterance", type: "TEXT" },
          config: [{ text: "static narration", type: "text" }],
          defaultContent: [{ text: "fallback narration", type: "text" }],
        },
      ],
      inputs: [{ state: "init", workType: "story" }],
      outputs: [{ state: "complete", workType: "story" }],
    });
    expect(updatedFactory?.workstations?.[1]).toEqual(
      factory.workstations?.[1],
    );
    expect(updatedFactory?.workers).toEqual(factory.workers);
  });

  it("converts a prompt-oriented workstation draft into INFERENCE_RUN on save", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "tts-factory",
      workers: [
        {
          name: "tts-worker",
          type: "MODEL_WORKER",
          operations: [
            {
              name: "TTS",
              inputs: [
                { name: "text", contentTypes: ["TEXT"], required: true },
              ],
              outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
            },
          ],
        },
      ],
      workstations: [
        {
          body: "Narrate the story.",
          name: "speak-story",
          type: "MODEL_WORKSTATION",
          worker: "tts-worker",
          inputs: [{ state: "init", workType: "story" }],
          outputs: [{ state: "complete", workType: "story" }],
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      {
        ...selectedNode,
        transition_id: "speak-story",
        workstation_name: "speak-story",
      },
      {
        behavior: "STANDARD",
        cron: null,
        guards: [],
        inputs: [{ guards: [], state: "init", workType: "story" }],
        name: "speak-story",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "",
            defaultContentText: "",
            selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
          },
        ],
        prompt: "Narrate the story.",
        runnerName: null,
        workerName: "tts-worker",
        workstationType: "INFERENCE_RUN",
      },
    );

    expect(updatedFactory?.workstations?.[0]).toEqual({
      name: "speak-story",
      type: "INFERENCE_RUN",
      worker: "tts-worker",
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          selector: { label: "utterance", type: "TEXT" },
        },
      ],
      inputs: [{ state: "init", workType: "story" }],
      outputs: [{ state: "complete", workType: "story" }],
    });
    expect(updatedFactory?.workstations?.[0]).not.toHaveProperty("body");
    expect(updatedFactory?.workstations?.[0]).not.toHaveProperty("runner");
  });

  it("defaults omitted workstation type to AGENT_RUN", () => {
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
    ).toBe("AGENT_RUN");
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
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {},
      operation: "",
      operationBindings: [],
      prompt: "Review the latest story changes before approval.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode", "pi"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "missing-worker",
      workerOptions: [],
      workerTypeByName: {},
      guards: [],
      inputs: [{ guards: [], state: "queued", workType: "story" }],
      workstationName: "Review",
      workstationOptions: ["Review"],
      workstationType: "AGENT_RUN",
      workstationTypeOptions: ["AGENT_RUN", "INFERENCE_RUN"],
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
        guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
        inputs: [{ guards: [], state: "queued", workType: "story" }],
        name: "Review",
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
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {
        processor: [],
      },
      operation: "",
      operationBindings: [],
      prompt: "Review work",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode", "pi"],
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
      guards: [],
      inputs: [{ guards: [], state: "queued", workType: "story" }],
      workstationName: "Review",
      workstationOptions: ["Review", "Plan", "Code"],
      workstationType: "AGENT_RUN",
      workstationTypeOptions: ["AGENT_RUN", "INFERENCE_RUN"],
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

    expect(
      resolveEditableWorkstationValues(factory, selectedNode),
    ).toMatchObject({
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
        guards: [],
        inputs: [{ guards: [], state: "queued", workType: "story" }],
        name: "Review",
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
        guards: [],
        inputs: [{ guards: [], state: "queued", workType: "story" }],
        name: "Review",
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
        guards: [],
        inputs: [{ guards: [], state: "queued", workType: "story" }],
        name: "Review",
        prompt: "Updated review work",
        runnerName: null,
        workerName: "missing-worker",
      }),
    ).toBeNull();
  });

  it("preserves legacy worker and ignores draft worker, prompt, and runner for LOGICAL_MOVE", () => {
    const logicalMoveNode: DashboardWorkstationNode = {
      model: "",
      node_id: "move",
      transition_id: "move",
      workstation_kind: "LOGICAL_MOVE",
      workstation_name: "Move",
    };
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workstations: [
        {
          body: "Legacy prompt that must not be overwritten.",
          id: "move",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Move",
          outputs: [{ state: "moved", workType: "story" }],
          runner: "gemini",
          type: "LOGICAL_MOVE",
          worker: "removed-worker",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      logicalMoveNode,
      {
        behavior: "POLLER",
        cron: null,
        guards: [],
        inputs: [],
        name: "Move",
        operation: "",
        operationBindings: [],
        prompt: "Draft prompt must not apply.",
        runnerName: "codex",
        workerName: "missing-worker",
        workstationType: "LOGICAL_MOVE",
      },
    );

    expect(updatedFactory).toMatchObject({
      workstations: [
        {
          body: "Legacy prompt that must not be overwritten.",
          name: "Move",
          runner: "gemini",
          type: "LOGICAL_MOVE",
          worker: "removed-worker",
        },
      ],
    });
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

    expect(
      resolveEditableWorkstationValues(factory, selectedNode),
    ).toMatchObject({
      behavior: "CRON",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
      guards: [],
      inputs: [],
    });
  });

  it("reads workstation-level and per-input guards into editable values", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          guards: [
            {
              matchConfig: { inputKey: ".Name" },
              type: "MATCHES_FIELDS",
            },
            {
              maxVisits: 2,
              type: "VISIT_COUNT",
              workstation: "Plan",
            },
          ],
          id: "review",
          inputs: [
            {
              guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode),
    ).toMatchObject({
      guards: [
        {
          matchConfig: { inputKey: ".Name" },
          type: "MATCHES_FIELDS",
        },
        {
          maxVisits: 2,
          type: "VISIT_COUNT",
          workstation: "Plan",
        },
      ],
      inputs: [
        {
          guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
          state: "queued",
          workType: "story",
        },
      ],
    });
  });

  it("preserves edited MATCHES_FIELDS inputKey verbatim when applying the workstation draft", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
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
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const editedSelector = '.Tags["_last_output"]';
    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        guards: [
          {
            matchConfig: { inputKey: editedSelector },
            type: "MATCHES_FIELDS",
          },
        ],
      },
    );

    expect(updatedFactory?.workstations?.[0]?.guards).toEqual([
      {
        matchConfig: { inputKey: editedSelector },
        type: "MATCHES_FIELDS",
      },
    ]);
  });

  it("writes draft guards onto the selected workstation", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
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
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        guards: [{ maxVisits: 3, type: "VISIT_COUNT", workstation: "Review" }],
        inputs: [
          {
            guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
            state: "queued",
            workType: "story",
          },
        ],
      },
    );

    expect(updatedFactory?.workstations?.[0]).toMatchObject({
      body: "Review work",
      guards: [{ maxVisits: 3, type: "VISIT_COUNT", workstation: "Review" }],
      inputs: [
        {
          guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
          state: "queued",
          workType: "story",
        },
      ],
    });
  });

  it("normalizes multiple input guards to one when reading editable values", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [
            {
              guards: [
                { matchInput: "planItem", type: "SAME_NAME" },
                { parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" },
              ],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          outputs: [],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.inputs,
    ).toEqual([
      {
        guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
        state: "queued",
        workType: "story",
      },
    ]);
  });

  it("applies SAME_NAME and ALL_CHILDREN_COMPLETE input guards onto the selected workstation", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [
            { state: "queued", workType: "planItem" },
            { state: "queued", workType: "story" },
          ],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        inputs: [
          {
            guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
            state: "queued",
            workType: "planItem",
          },
          {
            guards: [
              { parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" },
            ],
            state: "queued",
            workType: "story",
          },
        ],
      },
    );

    expect(updatedFactory?.workstations?.[0]?.inputs).toEqual([
      {
        guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
        state: "queued",
        workType: "planItem",
      },
      {
        guards: [{ parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" }],
        state: "queued",
        workType: "story",
      },
    ]);
  });

  it("clears an input guard without removing the input slot", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [
            {
              guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          outputs: [],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        inputs: [{ guards: [], state: "queued", workType: "story" }],
      },
    );

    expect(updatedFactory?.workstations?.[0]?.inputs).toEqual([
      { state: "queued", workType: "story" },
    ]);
  });

  it("preserves outputs, special IO, workstation guards, and other workstations when only input guards change", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [],
          worker: "reviewer",
        },
        {
          body: "Review work",
          guards: [{ maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" }],
          id: "review",
          inputs: [
            {
              guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          onRejection: [{ state: "rejected", workType: "story" }],
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        inputs: [
          {
            guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
            state: "queued",
            workType: "story",
          },
        ],
      },
    );

    expect(updatedFactory?.workstations?.[0]).toMatchObject({
      body: "Plan work",
      inputs: [{ state: "queued", workType: "story" }],
      name: "Plan",
    });
    expect(updatedFactory?.workstations?.[1]).toMatchObject({
      body: "Review work",
      guards: [{ maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" }],
      inputs: [
        {
          guards: [{ matchInput: "planItem", type: "SAME_TRACE_ID" }],
          state: "queued",
          workType: "story",
        },
      ],
      onRejection: [{ state: "rejected", workType: "story" }],
      outputs: [{ state: "approved", workType: "story" }],
    });
  });

  it("preserves workstation and input guards when only non-guard draft fields change", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          guards: [{ maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" }],
          id: "review",
          inputs: [
            {
              guards: [
                { parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" },
              ],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };
    const editableValues = resolveEditableWorkstationValues(
      factory,
      selectedNode,
    );
    expect(editableValues).not.toBeNull();

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        ...editableWorkstationDraftFromValues(
          editableValues as NonNullable<typeof editableValues>,
        ),
        prompt: "Updated review prompt only.",
        runnerName: "gemini",
      },
    );

    expect(updatedFactory?.workstations?.[0]).toMatchObject({
      body: "Updated review prompt only.",
      guards: [{ maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" }],
      inputs: [
        {
          guards: [{ parentInput: "planItem", type: "ALL_CHILDREN_COMPLETE" }],
          state: "queued",
          workType: "story",
        },
      ],
      runner: "gemini",
    });
  });
});

describe("applyEditableWorkstationDraft failures", () => {
  it("returns null when the selected workstation cannot be resolved", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [{ model: "gpt-5.5", name: "reviewer", type: "MODEL_WORKER" }],
      workstations: [],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        behavior: "STANDARD",
        cron: null,
        guards: [],
        inputs: [],
        name: "Review",
        prompt: "Updated prompt",
        runnerName: "gemini",
        workerName: "reviewer",
      }),
    ).toBeNull();
  });

  it("returns null when workers are missing for a model workstation draft", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workstations: [
        {
          body: "Review work",
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
      applyEditableWorkstationDraft(factory, selectedNode, {
        behavior: "STANDARD",
        cron: null,
        guards: [],
        inputs: [],
        name: "Review",
        prompt: "Updated prompt",
        runnerName: "gemini",
        workerName: "reviewer",
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

    expect(
      resolveEditableWorkstationValues(factory, selectedNode),
    ).toMatchObject({
      behavior: "CRON",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
      cron: {
        schedule: "0 * * * *",
        triggerAtStart: false,
        jitter: "",
        expiryWindow: "",
      },
    });
  });
});

describe("editable workstation cron draft", () => {
  const cronWorkstationNode: DashboardWorkstationNode = {
    model: "script",
    node_id: "cron-node",
    transition_id: "cron-node",
    workstation_kind: "MODEL_WORKSTATION",
    workstation_name: "Cron Tick",
  };

  const cronFactory: CanonicalFactoryDefinition = {
    name: "Cron Factory",
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
        cron: {
          expiryWindow: "30m",
          jitter: "5s",
          schedule: "0 9 * * *",
          triggerAtStart: true,
        },
        inputs: [],
        name: "Cron Tick",
        outputs: [],
        worker: "cron-runner",
      },
    ],
    workTypes: [],
  };

  it("resolves cron fields with defaults when optional values are absent", () => {
    expect(
      resolveEditableWorkstationCron({
        cron: { schedule: "*/15 * * * *" },
      }),
    ).toEqual({
      schedule: "*/15 * * * *",
      triggerAtStart: false,
      jitter: "",
      expiryWindow: "",
    });
  });

  it("builds editable drafts from cron values and detects cron dirty state", () => {
    const values = resolveEditableWorkstationValues(
      cronFactory,
      cronWorkstationNode,
    );
    expect(values?.cron).toEqual({
      schedule: "0 9 * * *",
      triggerAtStart: true,
      jitter: "5s",
      expiryWindow: "30m",
    });
    if (!values) {
      throw new Error("expected cron workstation values");
    }

    const draft = editableWorkstationDraftFromValues(values);
    expect(draft.cron).toEqual(values.cron);
    const draftCron = draft.cron ?? {
      expiryWindow: "",
      jitter: "",
      schedule: "",
      triggerAtStart: false,
    };
    expect(
      editableWorkstationDraftsEqual(draft, {
        ...draft,
        cron: {
          ...draftCron,
          schedule: "0 10 * * *",
        },
      }),
    ).toBe(false);
  });

  it("persists cron edits for CRON workstations and omits cron for non-CRON drafts", () => {
    const updatedCronFactory = applyEditableWorkstationDraft(
      cronFactory,
      cronWorkstationNode,
      {
        behavior: "CRON",
        cron: {
          schedule: "0 12 * * *",
          triggerAtStart: false,
          jitter: "1s",
          expiryWindow: "10m",
        },
        guards: [],
        inputs: [],
        name: "Cron Tick",
        prompt: "",
        runnerName: null,
        workerName: "cron-runner",
      },
    );

    expect(updatedCronFactory?.workstations?.[0]).toMatchObject({
      behavior: "CRON",
      cron: {
        schedule: "0 12 * * *",
        triggerAtStart: false,
        jitter: "1s",
        expiryWindow: "10m",
      },
    });

    const cronWorkstation = cronFactory.workstations?.[0];
    if (!cronWorkstation) {
      throw new Error("expected cron workstation fixture");
    }
    const standardFactory: CanonicalFactoryDefinition = {
      ...cronFactory,
      workstations: [
        {
          ...cronWorkstation,
          behavior: "STANDARD",
        },
      ],
    };

    const updatedStandardFactory = applyEditableWorkstationDraft(
      standardFactory,
      cronWorkstationNode,
      {
        behavior: "STANDARD",
        cron: {
          schedule: "0 12 * * *",
          triggerAtStart: true,
          jitter: "1s",
          expiryWindow: "10m",
        },
        guards: [],
        inputs: [],
        name: "Cron Tick",
        prompt: "Run on demand.",
        runnerName: null,
        workerName: "cron-runner",
      },
    );

    expect(updatedStandardFactory?.workstations?.[0]).toMatchObject({
      behavior: "STANDARD",
      body: "Run on demand.",
    });
    expect(updatedStandardFactory?.workstations?.[0]).not.toHaveProperty(
      "cron",
    );
  });

  it("detects external cron sub-field overwrites", () => {
    const sessionStartDraft = editableWorkstationDraftFromValues({
      behavior: "CRON",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
      cron: {
        schedule: "0 9 * * *",
        triggerAtStart: true,
        jitter: "5s",
        expiryWindow: "30m",
      },
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {},
      operation: "",
      operationBindings: [],
      prompt: null,
      resolvedRunnerSelection: { runnerId: "codex", source: "default" },
      runnerName: null,
      runnerOptions: ["codex"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "cron-runner",
      workerOptions: ["cron-runner"],
      workerTypeByName: { "cron-runner": "SCRIPT_WORKER" },
      guards: [],
      inputs: [],
      workstationName: "Cron Tick",
      workstationOptions: ["Cron Tick"],
      workstationType: "MODEL_WORKSTATION",
    });
    const sessionCron = sessionStartDraft.cron ?? {
      expiryWindow: "",
      jitter: "",
      schedule: "",
      triggerAtStart: false,
    };
    const draft = {
      ...sessionStartDraft,
      cron: {
        ...sessionCron,
        schedule: "0 10 * * *",
      },
    };
    const latestDefinitionDraft = {
      ...sessionStartDraft,
      cron: {
        ...sessionCron,
        jitter: "10s",
      },
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        sessionStartDraft,
        draft,
        latestDefinitionDraft,
      ),
    ).toEqual(["cronJitter"]);
  });
});

describe("editable workstation name draft", () => {
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
        body: "Review work",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        worker: "reviewer",
      },
      {
        body: "Plan work",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "approved", workType: "story" }],
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };

  it("initializes draft name from the selected workstation", () => {
    const values = resolveEditableWorkstationValues(factory, selectedNode);
    expect(values).not.toBeNull();
    expect(
      editableWorkstationDraftFromValues(values as NonNullable<typeof values>),
    ).toMatchObject({
      name: "Review",
    });
  });

  it("treats trimmed name-only changes as dirty and unchanged trims as clean", () => {
    const values = resolveEditableWorkstationValues(factory, selectedNode);
    expect(values).not.toBeNull();
    const draft = editableWorkstationDraftFromValues(
      values as NonNullable<typeof values>,
    );

    expect(
      editableWorkstationDraftsEqual(draft, { ...draft, name: "  Review  " }),
    ).toBe(true);
    expect(
      editableWorkstationDraftsEqual(draft, { ...draft, name: "Reviewed" }),
    ).toBe(false);
  });

  it("applies a trimmed rename onto the selected workstation in the pending factory", () => {
    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        behavior: "STANDARD",
        cron: null,
        guards: [],
        inputs: [{ guards: [], state: "queued", workType: "story" }],
        name: "  Reviewed  ",
        prompt: "Review work",
        runnerName: null,
        workerName: "reviewer",
      },
    );

    expect(updatedFactory?.workstations?.[0]?.name).toBe("Reviewed");
    expect(updatedFactory?.workstations?.[1]?.name).toBe("Plan");
  });

  it("rewrites VISIT_COUNT workstation references across the factory when renaming", () => {
    const factoryWithGuards: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        { model: "gpt-5.4", name: "Plan", type: "MODEL_WORKER" },
        { model: "gpt-5.5", name: "reviewer", type: "MODEL_WORKER" },
      ],
      workstations: [
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          worker: "reviewer",
        },
        {
          body: "Review work",
          guards: [
            { maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" },
            { maxVisits: 1, type: "VISIT_COUNT", workstation: "Review" },
          ],
          id: "review",
          inputs: [
            {
              guards: [
                { maxVisits: 1, type: "VISIT_COUNT", workstation: "Review" },
              ],
              state: "queued",
              workType: "story",
            },
          ],
          name: "Review",
          worker: "Plan",
        },
        {
          body: "Ship work",
          guards: [
            { maxVisits: 1, type: "VISIT_COUNT", workstation: "Review" },
          ],
          id: "ship",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Ship",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factoryWithGuards,
      selectedNode,
      {
        behavior: "STANDARD",
        cron: null,
        guards: [
          { maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" },
          { maxVisits: 1, type: "VISIT_COUNT", workstation: "Review" },
        ],
        inputs: [
          {
            guards: [
              { maxVisits: 1, type: "VISIT_COUNT", workstation: "Review" },
            ],
            state: "queued",
            workType: "story",
          },
        ],
        name: "Reviewed",
        prompt: "Review work",
        runnerName: null,
        workerName: "Plan",
      },
    );

    expect(updatedFactory?.workstations).toEqual([
      {
        body: "Plan work",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        worker: "reviewer",
      },
      {
        body: "Review work",
        guards: [
          { maxVisits: 2, type: "VISIT_COUNT", workstation: "Plan" },
          { maxVisits: 1, type: "VISIT_COUNT", workstation: "Reviewed" },
        ],
        id: "review",
        inputs: [
          {
            guards: [
              { maxVisits: 1, type: "VISIT_COUNT", workstation: "Reviewed" },
            ],
            state: "queued",
            workType: "story",
          },
        ],
        name: "Reviewed",
        worker: "Plan",
      },
      {
        body: "Ship work",
        guards: [
          { maxVisits: 1, type: "VISIT_COUNT", workstation: "Reviewed" },
        ],
        id: "ship",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Ship",
        worker: "reviewer",
      },
    ]);
  });

  it("includes name in overwrite-field detection when the latest definition diverges", () => {
    const sessionStartDraft = editableWorkstationDraftFromValues({
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD"],
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {},
      operation: "",
      operationBindings: [],
      prompt: "Review work",
      resolvedRunnerSelection: { runnerId: "codex", source: "default" },
      runnerName: null,
      runnerOptions: ["codex"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNames: [],
      sharedWorkerWorkstationNamesByWorkerName: {},
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: { reviewer: "MODEL_WORKER" },
      guards: [],
      inputs: [{ guards: [], state: "queued", workType: "story" }],
      workstationName: "Review",
      workstationOptions: ["Review", "Plan"],
      workstationType: "MODEL_WORKSTATION",
    });
    const latestDefinitionDraft = {
      ...sessionStartDraft,
      name: "Reviewed",
    };
    const draft = {
      ...sessionStartDraft,
      name: "Reviewed again",
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        sessionStartDraft,
        draft,
        latestDefinitionDraft,
      ),
    ).toContain("name");
  });

  it("includes model-invoke overwrite fields when the latest definition diverges", () => {
    const sessionStartDraft = editableWorkstationDraftFromValues({
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD"],
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      modelInvokeWorkerOptions: ["tts-worker"],
      modelOperationsByWorkerName: {
        "tts-worker": [
          {
            name: "TTS",
            inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
            outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
          },
        ],
      },
      operation: "",
      operationBindings: [],
      prompt: "Review work",
      resolvedRunnerSelection: { runnerId: "codex", source: "default" },
      runnerName: null,
      runnerOptions: ["codex"],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNames: [],
      sharedWorkerWorkstationNamesByWorkerName: {},
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: { reviewer: "MODEL_WORKER" },
      guards: [],
      inputs: [{ guards: [], state: "queued", workType: "story" }],
      workstationName: "Review",
      workstationOptions: ["Review"],
      workstationType: "MODEL_WORKSTATION",
    });
    const latestDefinitionDraft = {
      ...sessionStartDraft,
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          configText: "server",
          defaultContentText: "",
          selector: { label: "", role: "", slot: "", type: "" },
        },
      ],
      workerName: "tts-worker",
      workstationType: "MODEL_INVOKE" as const,
    };
    const draft = {
      ...sessionStartDraft,
      operation: "",
      operationBindings: [],
      workerName: "reviewer",
      workstationType: "MODEL_WORKSTATION" as const,
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        sessionStartDraft,
        draft,
        latestDefinitionDraft,
      ),
    ).toEqual(
      expect.arrayContaining([
        "workstationType",
        "operation",
        "operationBindings",
        "worker",
      ]),
    );
  });
});

describe("workstation taxonomy save projection", () => {
  it("round-trips new taxonomy workstation saves without downgrading to legacy names", () => {
    const taxonomyFactory: CanonicalFactoryDefinition = {
      name: "Legacy Factory",
      workers: [
        {
          model: "gpt-5",
          name: "writer",
          type: WorkerType.INFERENCE_WORKER,
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          body: "Draft the story.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "draft",
          outputs: [{ state: "done", workType: "story" }],
          type: WorkstationType.AGENT_RUN,
          worker: "writer",
        },
      ],
    };

    const values = resolveEditableWorkstationValues(taxonomyFactory, {
      node_id: "draft",
      transition_id: "draft",
      workstation_kind: WorkstationType.AGENT_RUN,
      workstation_name: "draft",
    });
    if (!values) {
      throw new Error("expected editable workstation values");
    }

    const draft = editableWorkstationDraftFromValues({
      ...values,
      workstationType: WorkstationType.INFERENCE_RUN,
      operation: "TTS",
      operationBindings: [],
    });
    const saved = applyEditableWorkstationDraft(
      taxonomyFactory,
      {
        node_id: "draft",
        transition_id: "draft",
        workstation_kind: WorkstationType.AGENT_RUN,
        workstation_name: "draft",
      },
      {
        ...draft,
        workstationType: WorkstationType.INFERENCE_RUN,
      },
    );

    expect(saved?.workstations?.[0]).toMatchObject({
      name: "draft",
      type: WorkstationType.INFERENCE_RUN,
      worker: "writer",
    });
  });
});
