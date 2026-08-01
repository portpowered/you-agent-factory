import { describe, expect, it } from "bun:test";

import {
  createEmptyEditableWorkstationCronDraft,
  type EditableWorkstationDraft,
  type EditableWorkstationValues,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  resolveDraftForBehaviorChange,
  updateEditableWorkstationCronDraft,
} from "./editable-workstation-cron-draft-mutators";

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  cron: null,
  guards: [],
  inputs: [],
  name: "review",
  prompt: "Review the story.",
  runnerName: null,
  workerName: "reviewer",
};

const selectedEditableValues: EditableWorkstationValues = {
  behavior: "STANDARD",
  behaviorOptions: ["STANDARD", "REPEATER", "POLLER", "CRON"],
  cron: {
    schedule: "0 * * * *",
    triggerAtStart: true,
    jitter: "30s",
    expiryWindow: "5m",
  },
  effectiveRunnerName: "codex",
  factoryRunnerName: null,
  prompt: "Review the story.",
  resolvedRunnerSelection: {
    runnerId: "codex",
    source: "default",
  },
  runnerName: null,
  runnerOptions: ["codex"],
  runnerSelectionSource: "default",
  sharedWorkerWorkstationNames: [],
  sharedWorkerWorkstationNamesByWorkerName: {},
  workerModelProvider: null,
  workerName: "reviewer",
  workerOptions: ["reviewer"],
  workerTypeByName: { reviewer: "SCRIPT_WORKER" },
  workstationName: "review",
  workstationType: "WORKSTATION",
};

describe("resolveDraftForBehaviorChange", () => {
  it("initializes cron when switching to CRON without an existing draft cron", () => {
    const result = resolveDraftForBehaviorChange(
      baseDraft,
      "CRON",
      selectedEditableValues,
    );

    expect(result).toEqual({
      ...baseDraft,
      behavior: "CRON",
      cron: {
        schedule: "0 * * * *",
        triggerAtStart: true,
        jitter: "30s",
        expiryWindow: "5m",
      },
    });
  });

  it("uses an empty cron draft when switching to CRON and the factory has no cron", () => {
    const result = resolveDraftForBehaviorChange(baseDraft, "CRON", {
      ...selectedEditableValues,
      cron: null,
    });

    expect(result.cron).toEqual(createEmptyEditableWorkstationCronDraft());
  });

  it("preserves an in-progress cron draft when switching to CRON", () => {
    const draftWithCron: EditableWorkstationDraft = {
      ...baseDraft,
      cron: {
        schedule: "*/15 * * * *",
        triggerAtStart: false,
        jitter: "",
        expiryWindow: "",
      },
    };

    const result = resolveDraftForBehaviorChange(
      draftWithCron,
      "CRON",
      selectedEditableValues,
    );

    expect(result.cron).toEqual(draftWithCron.cron);
  });

  it("clears cron when switching away from CRON", () => {
    const cronDraft: EditableWorkstationDraft = {
      ...baseDraft,
      behavior: "CRON",
      cron: createEmptyEditableWorkstationCronDraft(),
    };

    const result = resolveDraftForBehaviorChange(
      cronDraft,
      "STANDARD",
      selectedEditableValues,
    );

    expect(result).toEqual({
      ...cronDraft,
      behavior: "STANDARD",
      cron: null,
    });
  });
});

describe("updateEditableWorkstationCronDraft", () => {
  it("returns the draft unchanged for non-CRON behavior", () => {
    expect(
      updateEditableWorkstationCronDraft(baseDraft, {
        schedule: "*/5 * * * *",
      }),
    ).toBe(baseDraft);
  });

  it("creates and patches cron fields for CRON behavior", () => {
    const cronDraft: EditableWorkstationDraft = {
      ...baseDraft,
      behavior: "CRON",
      cron: null,
    };

    expect(
      updateEditableWorkstationCronDraft(cronDraft, {
        schedule: "*/10 * * * *",
        triggerAtStart: true,
      }),
    ).toEqual({
      ...cronDraft,
      cron: {
        ...createEmptyEditableWorkstationCronDraft(),
        schedule: "*/10 * * * *",
        triggerAtStart: true,
      },
    });
  });
});
