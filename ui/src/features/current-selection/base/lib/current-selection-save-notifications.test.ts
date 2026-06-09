import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import { buildGraphSaveErrorToastDescription } from "../../../factory-graph-editor/lib/document-save/graph-document-save-notifications";
import { buildCurrentSelectionSaveSuccessStableIdentity } from "../../../notifications/lib/save-notification-delivery-policy";
import { resolveCurrentSelectionSaveToastNotification } from "./current-selection-save-notifications";

const messages = {
  saveFailedAffectedSummary: (labels: string) => `Affected: ${labels}`,
  saveFailedTitle: "Worker save failed",
  saveSuccessDescription:
    "Running factory saved. worker-1 was updated in the running factory definition.",
  saveSuccessTitle: "Worker saved",
  staleVersionDetail:
    "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
};

const formatAffectedSummary = messages.saveFailedAffectedSummary;

describe("buildCurrentSelectionSaveSuccessStableIdentity", () => {
  it("returns per-entity stable ids distinct from graph-save-success", () => {
    expect(buildCurrentSelectionSaveSuccessStableIdentity("worker")).toEqual({
      kind: "success",
      stableId: "worker-save-success",
    });
    expect(
      buildCurrentSelectionSaveSuccessStableIdentity("workstation"),
    ).toEqual({
      kind: "success",
      stableId: "workstation-save-success",
    });
    expect(buildCurrentSelectionSaveSuccessStableIdentity("resource")).toEqual({
      kind: "success",
      stableId: "resource-save-success",
    });
    expect(buildCurrentSelectionSaveSuccessStableIdentity("work-type")).toEqual(
      {
        kind: "success",
        stableId: "work-type-save-success",
      },
    );
    expect(
      buildCurrentSelectionSaveSuccessStableIdentity("work-state"),
    ).toEqual({
      kind: "success",
      stableId: "work-state-save-success",
    });
    expect(buildCurrentSelectionSaveSuccessStableIdentity("doc")).toEqual({
      kind: "success",
      stableId: "doc-save-success",
    });
  });
});

describe("resolveCurrentSelectionSaveToastNotification success and idle", () => {
  it("returns success toast when scoped save succeeded and the draft is clean", () => {
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: { status: "success" },
        entityKind: "worker",
        hasDraftChanges: false,
        messages,
        saveMutationError: null,
      }),
    ).toEqual({
      description: messages.saveSuccessDescription,
      entityKind: "worker",
      key: "success",
      kind: "success",
      title: messages.saveSuccessTitle,
    });
  });

  it("suppresses success toast while draft changes remain", () => {
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: { status: "success" },
        entityKind: "worker",
        hasDraftChanges: true,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
  });

  it("returns null for idle, confirming, and submitting save states", () => {
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: { status: "idle" },
        entityKind: "worker",
        hasDraftChanges: false,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: { status: "confirming" },
        entityKind: "worker",
        hasDraftChanges: false,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: { status: "submitting" },
        entityKind: "worker",
        hasDraftChanges: false,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
  });
});

describe("resolveCurrentSelectionSaveToastNotification errors", () => {
  it("returns warning toast for stale version save failures with stale-version detail", () => {
    const saveMutationError = new CurrentFactoryDefinitionError(
      "The factory definition changed on the server.",
      {
        code: "STALE_FACTORY_VERSION",
        targets: [],
      },
    );

    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: {
          message: saveMutationError.message,
          status: "warning",
        },
        entityKind: "worker",
        hasDraftChanges: true,
        messages,
        saveMutationError,
      }),
    ).toEqual({
      description: messages.staleVersionDetail,
      entityKind: "worker",
      key: `warning:${saveMutationError.message}`,
      kind: "warning",
      title: saveMutationError.message,
    });
  });

  it("returns error toast with validation target summary when targets are present", () => {
    const targets = [
      workerFieldValidationTarget("prompt", "Prompt is required."),
      workerFieldValidationTarget("kind", "Kind is required."),
    ];
    const saveMutationError = new CurrentFactoryDefinitionError(
      "Factory definition is invalid.",
      {
        code: "INVALID_FACTORY_DEFINITION",
        targets,
      },
    );

    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: {
          errorMessage: saveMutationError.message,
          status: "error",
        },
        entityKind: "worker",
        hasDraftChanges: true,
        messages,
        saveMutationError,
      }),
    ).toEqual({
      description: buildGraphSaveErrorToastDescription(
        saveMutationError.message,
        targets,
        formatAffectedSummary,
      ),
      entityKind: "worker",
      key: `error:${saveMutationError.message}:${buildGraphSaveErrorToastDescription(
        saveMutationError.message,
        targets,
        formatAffectedSummary,
      )}`,
      kind: "error",
      title: messages.saveFailedTitle,
    });
  });

  it("returns error toast with only the error message when no targets are present", () => {
    expect(
      resolveCurrentSelectionSaveToastNotification({
        documentSave: {
          errorMessage: "Network dropped",
          status: "error",
        },
        entityKind: "workstation",
        hasDraftChanges: true,
        messages: {
          ...messages,
          saveFailedTitle: "Workstation save failed",
        },
        saveMutationError: null,
      }),
    ).toEqual({
      description: "Network dropped",
      entityKind: "workstation",
      key: "error:Network dropped:Network dropped",
      kind: "error",
      title: "Workstation save failed",
    });
  });
});
