import { describe, expect, it, vi } from "vitest";

import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import {
  editableWorkstationDraftFromValues,
  resolveEditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { validateEditableWorkstationDraft } from "../lib/validation/editable-workstation-configuration-validation";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import { buildReadyEditableWorkstationConfigurationState } from "./editable-workstation-ready-configuration-state";

const messages = getWorkstationDetailMessages();

const cronWorkstationNode: DashboardWorkstationNode = {
  model: "script",
  node_id: "cron-node",
  transition_id: "cron-node",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Cron Tick",
};

const cronFactory: CurrentFactoryDocument = {
  name: "Cron Factory",
  runner: "codex",
  version: { logical: "1", physical: "2026-06-01T00:00:00Z" },
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
      body: "Run the nightly refresh.",
      cron: {
        expiryWindow: "30m",
        jitter: "5s",
        schedule: "0 9 * * *",
        triggerAtStart: true,
      },
      id: "cron-tick",
      inputs: [{ state: "queued", workType: "story" }],
      name: "Cron Tick",
      outputs: [{ state: "done", workType: "story" }],
      promptFile: "prompts/cron.md",
      runner: "codex",
      worker: "cron-runner",
    },
  ],
  workTypes: [],
};

type SessionState = {
  draft: ReturnType<typeof editableWorkstationDraftFromValues>;
  latestDefinitionDraft: ReturnType<typeof editableWorkstationDraftFromValues>;
  sessionStartDraft: ReturnType<typeof editableWorkstationDraftFromValues>;
};

function buildReadyHarness() {
  const selectedEditableValues = resolveEditableWorkstationValues(
    cronFactory,
    cronWorkstationNode,
  );
  if (!selectedEditableValues) {
    throw new Error("expected cron workstation editable values");
  }

  const baseDraft = editableWorkstationDraftFromValues(selectedEditableValues);
  let sessionState: SessionState = {
    draft: baseDraft,
    latestDefinitionDraft: baseDraft,
    sessionStartDraft: baseDraft,
  };

  const setSessionState = vi.fn(
    (updater: (current: SessionState | null) => SessionState | null) => {
      sessionState = updater(sessionState) ?? sessionState;
    },
  );

  const buildReady = (draft = sessionState.draft) => {
    const resolvedValidationErrors = validateEditableWorkstationDraft(
      draft,
      selectedEditableValues,
      {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      },
      messages,
    );

    return buildReadyEditableWorkstationConfigurationState({
      editableDefinition: cronFactory,
      messages,
      promptHelpState: { status: "empty", message: "No prompt help." },
      promptValidationState: {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      },
      resolvedValidationErrors,
      selectedEditableValues,
      selectedNode: cronWorkstationNode,
      sessionState,
      setSessionState,
    });
  };

  return {
    buildReady,
    getSessionState: () => sessionState,
    selectedEditableValues,
  };
}

describe("buildReadyEditableWorkstationConfigurationState cron handlers", () => {
  it("updates cron draft fields through cron handlers", () => {
    const { buildReady, getSessionState } = buildReadyHarness();
    const readyState = buildReady();

    readyState.onCronScheduleChange("*/5 * * * *");
    readyState.onCronJitterChange("10s");
    readyState.onCronExpiryWindowChange("1h");
    readyState.onCronTriggerAtStartChange(false);

    expect(getSessionState().draft.cron).toEqual({
      expiryWindow: "1h",
      jitter: "10s",
      schedule: "*/5 * * * *",
      triggerAtStart: false,
    });
    expect(buildReady().isDirty).toBe(true);
  });
});

describe("buildReadyEditableWorkstationConfigurationState session lifecycle", () => {
  it("clears dirty state after markChangesSaved", () => {
    const { buildReady, getSessionState } = buildReadyHarness();
    const readyState = buildReady();

    readyState.onCronScheduleChange("0 0 * * *");
    readyState.markChangesSaved();

    expect(getSessionState().sessionStartDraft.cron?.schedule).toBe(
      "0 0 * * *",
    );
    expect(buildReady().isDirty).toBe(false);
  });

  it("restores the latest definition draft on reset", () => {
    const { buildReady, getSessionState, selectedEditableValues } =
      buildReadyHarness();
    const readyState = buildReady();
    const latestDraft = editableWorkstationDraftFromValues(
      selectedEditableValues,
    );

    readyState.onCronScheduleChange("0 12 * * *");
    readyState.onResetToLatest();

    expect(getSessionState().draft).toEqual(latestDraft);
    expect(getSessionState().sessionStartDraft).toEqual(latestDraft);
    expect(buildReady().isDirty).toBe(false);
  });
});

describe("buildReadyEditableWorkstationConfigurationState save projection", () => {
  it("returns pendingFactoryDefinition with updated cron when validation passes", () => {
    const { buildReady, getSessionState } = buildReadyHarness();
    const readyState = buildReady();

    readyState.onCronScheduleChange("0 8 * * *");

    const pending = buildReady(
      getSessionState().draft,
    ).pendingFactoryDefinition;
    expect(pending?.workstations?.[0]?.cron).toEqual({
      expiryWindow: "30m",
      jitter: "5s",
      schedule: "0 8 * * *",
      triggerAtStart: true,
    });
  });

  it("omits pendingFactoryDefinition when validation errors exist", () => {
    const { buildReady, getSessionState } = buildReadyHarness();
    const readyState = buildReady();

    readyState.onCronScheduleChange("");

    const invalidReady = buildReadyEditableWorkstationConfigurationState({
      editableDefinition: cronFactory,
      messages,
      promptHelpState: { status: "empty", message: "No prompt help." },
      promptValidationState: {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      },
      resolvedValidationErrors: { cronSchedule: "Schedule is required." },
      selectedEditableValues: buildReadyHarness().selectedEditableValues,
      selectedNode: cronWorkstationNode,
      sessionState: getSessionState(),
      setSessionState: vi.fn(),
    });

    expect(invalidReady.pendingFactoryDefinition).toBeNull();
    expect(invalidReady.hasValidationErrors).toBe(true);
  });

  it("omits pendingFactoryDefinition while prompt validation is loading without marking the draft invalid", () => {
    const { buildReady, getSessionState, selectedEditableValues } =
      buildReadyHarness();
    const readyState = buildReady();

    readyState.onBehaviorChange("STANDARD");
    readyState.onPromptChange("Review {{ .WorkID }}");

    const loadingReady = buildReadyEditableWorkstationConfigurationState({
      editableDefinition: cronFactory,
      messages,
      promptHelpState: { status: "empty", message: "No prompt help." },
      promptValidationState: { status: "loading" },
      resolvedValidationErrors: {},
      selectedEditableValues,
      selectedNode: cronWorkstationNode,
      sessionState: getSessionState(),
      setSessionState: vi.fn(),
    });

    expect(loadingReady.pendingFactoryDefinition).toBeNull();
    expect(loadingReady.hasValidationErrors).toBe(false);
  });
});

describe("buildReadyEditableWorkstationConfigurationState workstation options", () => {
  it("exposes ready workstation options for guard editors", () => {
    const { buildReady } = buildReadyHarness();

    expect(buildReady().workstationOptionsState).toEqual({
      options: ["Cron Tick"],
      status: "ready",
    });
  });
});

const modelInvokeWorkstationNode: DashboardWorkstationNode = {
  model: "script",
  node_id: "speak-node",
  transition_id: "speak-node",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Speak Story",
};

const modelInvokeFactory: CurrentFactoryDocument = {
  name: "Invoke Factory",
  runner: "codex",
  version: { logical: "1", physical: "2026-06-01T00:00:00Z" },
  workers: [
    {
      name: "tts-worker",
      type: "MODEL_WORKER",
      operations: [
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
          outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
        },
        {
          name: "STT",
          inputs: [{ name: "audio", contentTypes: ["AUDIO"], required: true }],
          outputs: [{ name: "text", contentTypes: ["TEXT"] }],
        },
      ],
    },
  ],
  workstations: [
    {
      behavior: "STANDARD",
      body: "Speak the story aloud.",
      id: "speak-story",
      inputs: [{ state: "queued", workType: "story" }],
      name: "Speak Story",
      outputs: [{ state: "done", workType: "story" }],
      promptFile: "prompts/speak.md",
      runner: "codex",
      worker: "tts-worker",
    },
  ],
  workTypes: [],
};

function buildModelInvokeReadyHarness() {
  const selectedEditableValues = resolveEditableWorkstationValues(
    modelInvokeFactory,
    modelInvokeWorkstationNode,
  );
  if (!selectedEditableValues) {
    throw new Error("expected model invoke workstation editable values");
  }

  const baseDraft = editableWorkstationDraftFromValues(selectedEditableValues);
  let sessionState: SessionState = {
    draft: baseDraft,
    latestDefinitionDraft: baseDraft,
    sessionStartDraft: baseDraft,
  };

  const setSessionState = vi.fn(
    (updater: (current: SessionState | null) => SessionState | null) => {
      sessionState = updater(sessionState) ?? sessionState;
    },
  );

  const buildReady = (draft = sessionState.draft) => {
    const resolvedValidationErrors = validateEditableWorkstationDraft(
      draft,
      selectedEditableValues,
      {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      },
      messages,
    );

    return buildReadyEditableWorkstationConfigurationState({
      editableDefinition: modelInvokeFactory,
      messages,
      promptHelpState: { status: "empty", message: "No prompt help." },
      promptValidationState: {
        diagnostics: [],
        result: { diagnostics: [], valid: true },
        status: "ready",
      },
      resolvedValidationErrors,
      selectedEditableValues,
      selectedNode: modelInvokeWorkstationNode,
      sessionState,
      setSessionState,
    });
  };

  return {
    buildReady,
    getSessionState: () => sessionState,
    selectedEditableValues,
  };
}

describe("buildReadyEditableWorkstationConfigurationState model invoke handlers", () => {
  it("converts prompt-oriented drafts into model invoke configuration", () => {
    const { buildReady, getSessionState } = buildModelInvokeReadyHarness();
    const readyState = buildReady();

    readyState.onWorkstationTypeChange("MODEL_INVOKE");
    readyState.onWorkerChange("tts-worker");
    readyState.onOperationChange("STT");
    readyState.onOperationBindingsChange([
      {
        slot: "audio",
        configText: "",
        defaultContentText: "",
        selector: {
          label: "clip",
          role: "",
          slot: "input.audio",
          type: "AUDIO",
        },
      },
    ]);
    readyState.onNameChange("Speak Story Invoke");

    expect(getSessionState().draft).toMatchObject({
      name: "Speak Story Invoke",
      operation: "STT",
      workerName: "tts-worker",
      workstationType: "MODEL_INVOKE",
      operationBindings: [
        expect.objectContaining({
          slot: "audio",
          selector: expect.objectContaining({ label: "clip", type: "AUDIO" }),
        }),
      ],
    });
    expect(buildReady().operationOptionsState).toEqual({
      operations: expect.any(Array),
      options: ["TTS", "STT"],
      status: "ready",
    });
  });

  it("keeps prompt-oriented worker changes outside model invoke mode", () => {
    const { buildReady, getSessionState } = buildModelInvokeReadyHarness();
    const readyState = buildReady();

    readyState.onWorkerChange("tts-worker");
    readyState.onPromptChange("Updated prompt body.");
    readyState.onRunnerChange("reviewer");
    readyState.onBehaviorChange("STANDARD");

    expect(getSessionState().draft).toMatchObject({
      behavior: "STANDARD",
      prompt: "Updated prompt body.",
      runnerName: "reviewer",
      workerName: "tts-worker",
      workstationType: "AGENT_RUN",
    });
  });
});
