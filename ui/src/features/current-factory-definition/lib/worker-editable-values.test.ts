import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  areEditableWorkerDraftsEqual,
  EDITABLE_EXECUTOR_PROVIDERS,
  EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
  editableWorkerDraftFromValues,
  parseWorkerArgsText,
  resolveEditableWorkerValues,
} from "./worker-editable-values";

describe("resolveEditableWorkerValues for model workers", () => {
  it("joins the selected worker with referencing workstation names", () => {
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
        { id: "review", name: "Review", worker: "reviewer" },
        { id: "plan", name: "Plan", worker: "reviewer" },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkerValues(factory, "reviewer")).toEqual({
      args: [],
      ...EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      provider: null,
      skipPermissions: null,
      stopToken: null,
      timeout: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames: ["Review", "Plan"],
    });
  });

  it("initializes timeout from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          timeout: "5m",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.timeout).toBe("5m");
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values)).toMatchObject({
      timeoutAmount: "5",
      timeoutUnit: "m",
    });
  });

  it("initializes stopToken from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          stopToken: "<COMPLETE>",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.stopToken).toBe("<COMPLETE>");
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values).stopToken).toBe("<COMPLETE>");
  });

  it("initializes skipPermissions from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          skipPermissions: true,
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.skipPermissions).toBe(true);
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values).skipPermissions).toBe(true);
  });

  it("returns null when the worker is missing from the factory document", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workTypes: [],
    };

    expect(resolveEditableWorkerValues(factory, "missing-worker")).toBeNull();
  });
});

describe("resolveEditableWorkerValues for hosted Linear workers", () => {
  it("reads hosted Linear poller config with stable defaults for omitted optional values", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "secrets/linear-api-key" },
          linear: {
            claim: { assigneeField: "assignee.email" },
            mapping: { state: "queued", workType: "story" },
            pollInterval: "30s",
            stateIds: ["state-b"],
            teamIds: ["team-a"],
          },
          name: "linear-poller",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "linear-poller");
    expect(values).toEqual({
      args: [],
      authSecretRef: "secrets/linear-api-key",
      body: null,
      command: null,
      executorProvider: null,
      linearClaimAssigneeField: "assignee.email",
      linearClaimPresent: true,
      linearMappingState: "queued",
      linearMappingWorkType: "story",
      linearPollInterval: "30s",
      linearStateIds: ["state-b"],
      linearTeamIds: ["team-a"],
      model: null,
      modelLocality: null,
      modelProvider: null,
      provider: "LINEAR",
      skipPermissions: null,
      stopToken: null,
      timeout: null,
      type: "HOSTED_WORKER",
      workerName: "linear-poller",
      workstationNames: [],
    });

    if (!values) {
      throw new Error("expected linear-poller values");
    }
    expect(editableWorkerDraftFromValues(values)).toMatchObject({
      authSecretRef: "secrets/linear-api-key",
      linearClaimAssigneeField: "assignee.email",
      linearMappingState: "queued",
      linearMappingWorkType: "story",
      linearPollInterval: "30s",
      linearStateIdsText: "state-b",
      linearTeamIdsText: "team-a",
      provider: "LINEAR",
      type: "HOSTED_WORKER",
    });
  });
});

describe("areEditableWorkerDraftsEqual", () => {
  it("treats hosted Linear field changes as dirty", () => {
    const base = editableWorkerDraftFromValues({
      args: [],
      authSecretRef: "secrets/linear-api-key",
      body: null,
      command: null,
      executorProvider: null,
      linearClaimAssigneeField: null,
      linearClaimPresent: false,
      linearMappingState: "queued",
      linearMappingWorkType: "story",
      linearPollInterval: "30s",
      linearStateIds: ["state-b"],
      linearTeamIds: ["team-a"],
      model: null,
      modelLocality: null,
      modelProvider: null,
      provider: "LINEAR",
      skipPermissions: null,
      stopToken: null,
      timeout: null,
      type: "HOSTED_WORKER",
      workerName: "linear-poller",
      workstationNames: [],
    });

    expect(
      areEditableWorkerDraftsEqual(base, {
        ...base,
        authSecretRef: "secrets/other-key",
      }),
    ).toBe(false);
    expect(
      areEditableWorkerDraftsEqual(base, {
        ...base,
        linearPollInterval: "1m",
      }),
    ).toBe(false);
    expect(
      areEditableWorkerDraftsEqual(base, {
        ...base,
        linearTeamIdsText: "team-c",
      }),
    ).toBe(false);
    expect(
      areEditableWorkerDraftsEqual(base, {
        ...base,
        linearMappingWorkType: "task",
      }),
    ).toBe(false);
    expect(
      areEditableWorkerDraftsEqual(base, {
        ...base,
        linearClaimAssigneeField: "assignee.id",
      }),
    ).toBe(false);
    expect(areEditableWorkerDraftsEqual(base, base)).toBe(true);
  });
});

describe("parseWorkerArgsText", () => {
  it("splits args on newlines and drops blank lines", () => {
    expect(parseWorkerArgsText(" --verbose \n\n--dry-run\n")).toEqual([
      "--verbose",
      "--dry-run",
    ]);
  });
});

describe("editable worker provider catalogs", () => {
  it("exposes script-wrap as the executor provider option", () => {
    expect(EDITABLE_EXECUTOR_PROVIDERS).toEqual(["SCRIPT_WRAP"]);
  });
});
