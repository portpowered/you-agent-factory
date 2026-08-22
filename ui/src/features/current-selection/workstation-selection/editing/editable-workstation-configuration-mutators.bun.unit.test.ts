import { describe, expect, it, mock } from "bun:test";

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
  name: "nightly",
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

type SessionState = {
  draft: EditableWorkstationDraft;
  latestDefinitionDraft: EditableWorkstationDraft;
  sessionStartDraft: EditableWorkstationDraft;
};

function buildMutatorHarness(initialState: SessionState) {
  let sessionState = initialState;
  const setSessionState = mock(
    (updater: (current: SessionState | null) => SessionState | null) => {
      sessionState = updater(sessionState) ?? sessionState;
    },
  );
  const mutators = buildEditableWorkstationConfigurationMutators({
    selectedEditableValues,
    setSessionState,
  });

  return {
    getSessionState: () => sessionState,
    mutators,
    setSessionState,
  };
}

describe("buildEditableWorkstationConfigurationMutators cron fields", () => {
  it("updates cron draft fields through cron mutators", () => {
    const { getSessionState, mutators } = buildMutatorHarness({
      draft: cronDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    });

    mutators.onCronScheduleChange("0 9 * * *");
    mutators.onCronJitterChange("15s");
    mutators.onCronExpiryWindowChange("2m");
    mutators.onCronTriggerAtStartChange(false);

    expect(getSessionState().draft.cron).toEqual({
      expiryWindow: "2m",
      jitter: "15s",
      schedule: "0 9 * * *",
      triggerAtStart: false,
    });
  });
});

describe("buildEditableWorkstationConfigurationMutators session sync", () => {
  it("marks changes saved from the current draft", () => {
    const dirtyDraft: EditableWorkstationDraft = {
      ...cronDraft,
      cron: {
        ...baseCron,
        schedule: "0 9 * * *",
      },
    };
    const { getSessionState, mutators } = buildMutatorHarness({
      draft: dirtyDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    });

    mutators.markChangesSaved();
    expect(getSessionState().latestDefinitionDraft).toBe(dirtyDraft);
    expect(getSessionState().sessionStartDraft).toBe(dirtyDraft);
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
    const { getSessionState, mutators } = buildMutatorHarness({
      draft: dirtyDraft,
      latestDefinitionDraft,
      sessionStartDraft: cronDraft,
    });

    mutators.onResetToLatest();
    expect(getSessionState().draft).toBe(latestDefinitionDraft);
    expect(getSessionState().sessionStartDraft).toBe(latestDefinitionDraft);
  });
});

describe("buildEditableWorkstationConfigurationMutators other draft fields", () => {
  it("updates prompt, runner, worker, and behavior on the session draft", () => {
    const { getSessionState, mutators } = buildMutatorHarness({
      draft: cronDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    });

    mutators.onPromptChange("Updated prompt body.");
    mutators.onRunnerChange(null);
    mutators.onWorkerChange("operator");
    mutators.onBehaviorChange("STANDARD");

    expect(getSessionState().draft).toEqual({
      behavior: "STANDARD",
      cron: null,
      name: "nightly",
      prompt: "Updated prompt body.",
      runnerName: null,
      workerName: "operator",
    });
  });

  it("updates the workstation name on the session draft", () => {
    const { getSessionState, mutators } = buildMutatorHarness({
      draft: cronDraft,
      latestDefinitionDraft: cronDraft,
      sessionStartDraft: cronDraft,
    });

    mutators.onNameChange("nightly-refresh");

    expect(getSessionState().draft.name).toBe("nightly-refresh");
  });

  it("no-ops mutators when session state is null", () => {
    const setSessionState = mock((updater: (current: null) => null) =>
      updater(null),
    );
    const mutators = buildEditableWorkstationConfigurationMutators({
      selectedEditableValues,
      setSessionState,
    });

    mutators.onCronScheduleChange("0 0 * * *");
    mutators.onPromptChange("ignored");
    mutators.onRunnerChange("codex");
    mutators.onWorkerChange("ignored");
    mutators.onBehaviorChange("CRON");
    mutators.markChangesSaved();
    mutators.onResetToLatest();

    expect(setSessionState).toHaveBeenCalledTimes(7);
    for (const call of setSessionState.mock.calls) {
      expect(call[0](null)).toBeNull();
    }
  });
});
