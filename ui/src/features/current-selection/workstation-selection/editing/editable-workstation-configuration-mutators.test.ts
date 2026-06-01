import { describe, expect, it, vi } from "vitest";

import type {
  EditableWorkstationDraft,
  EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import { buildEditableWorkstationConfigurationMutators } from "./editable-workstation-configuration-mutators";

const baseCron = {
  expiryWindow: "30s",
  jitter: "5s",
  schedule: "0 * * * *",
  triggerAtStart: true,
};

const cronDraft: EditableWorkstationDraft = {
  behavior: "CRON",
  cron: baseCron,
  prompt: "Run the nightly refresh.",
  runnerName: "codex",
  workerName: "reviewer",
};

const selectedEditableValues: EditableWorkstationValues = {
  behavior: "CRON",
  behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
  cron: {
    expiryWindow: "1m",
    jitter: "10s",
    schedule: "*/15 * * * *",
    triggerAtStart: false,
  },
  effectiveRunnerName: "codex",
  factoryRunnerName: null,
  prompt: "Run the nightly refresh.",
  resolvedRunnerSelection: {
    runnerId: "codex",
    source: "workstation",
  },
  runnerName: "codex",
  runnerOptions: ["codex"],
  runnerSelectionSource: "workstation",
  sharedWorkerWorkstationNames: [],
  sharedWorkerWorkstationNamesByWorkerName: {},
  workerModelProvider: null,
  workerName: "reviewer",
  workerOptions: ["reviewer"],
  workerTypeByName: { reviewer: "SCRIPT_WORKER" },
  workstationName: "nightly",
  workstationType: "WORKSTATION",
};

describe("buildEditableWorkstationConfigurationMutators", () => {
  it("updates cron draft fields through cron mutators", () => {
    let sessionState = {
      draft: cronDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    };
    const setSessionState = vi.fn(
      (
        updater: (
          current: typeof sessionState | null,
        ) => typeof sessionState | null,
      ) => {
        sessionState = updater(sessionState) ?? sessionState;
      },
    );

    const mutators = buildEditableWorkstationConfigurationMutators({
      selectedEditableValues,
      setSessionState,
    });

    mutators.onCronScheduleChange("0 9 * * *");
    mutators.onCronJitterChange("15s");
    mutators.onCronExpiryWindowChange("2m");
    mutators.onCronTriggerAtStartChange(false);

    expect(sessionState.draft.cron).toEqual({
      expiryWindow: "2m",
      jitter: "15s",
      schedule: "0 9 * * *",
      triggerAtStart: false,
    });
  });

  it("marks changes saved from the current draft", () => {
    const dirtyDraft: EditableWorkstationDraft = {
      ...cronDraft,
      cron: {
        ...baseCron,
        schedule: "0 9 * * *",
      },
    };
    let sessionState = {
      draft: dirtyDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    };
    const setSessionState = vi.fn(
      (
        updater: (
          current: typeof sessionState | null,
        ) => typeof sessionState | null,
      ) => {
        sessionState = updater(sessionState) ?? sessionState;
      },
    );

    const mutators = buildEditableWorkstationConfigurationMutators({
      selectedEditableValues,
      setSessionState,
    });

    mutators.markChangesSaved();
    expect(sessionState.latestDefinitionDraft).toBe(dirtyDraft);
    expect(sessionState.sessionStartDraft).toBe(dirtyDraft);
  });

  it("resets the draft to the latest factory definition", () => {
    const latestDefinitionDraft: EditableWorkstationDraft = {
      ...cronDraft,
      cron: {
        ...baseCron,
        schedule: "*/5 * * * *",
      },
    };
    const dirtyDraft: EditableWorkstationDraft = {
      ...cronDraft,
      cron: {
        ...baseCron,
        schedule: "0 9 * * *",
      },
    };
    let sessionState = {
      draft: dirtyDraft,
      latestDefinitionDraft,
      sessionStartDraft: cronDraft,
    };
    const setSessionState = vi.fn(
      (
        updater: (
          current: typeof sessionState | null,
        ) => typeof sessionState | null,
      ) => {
        sessionState = updater(sessionState) ?? sessionState;
      },
    );

    const mutators = buildEditableWorkstationConfigurationMutators({
      selectedEditableValues,
      setSessionState,
    });

    mutators.onResetToLatest();
    expect(sessionState.draft).toBe(latestDefinitionDraft);
    expect(sessionState.sessionStartDraft).toBe(latestDefinitionDraft);
  });
});
