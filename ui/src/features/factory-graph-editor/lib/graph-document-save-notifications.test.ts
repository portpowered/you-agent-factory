import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../testing/factory-validation-target-fixtures";
import { STALE_FACTORY_GRAPH_DRAFT_WARNING } from "./graph-document-save-state";
import {
  buildGraphSaveErrorToastDescription,
  buildStaleVersionToastDescription,
  formatGraphSaveValidationTargetSummary,
  resolveGraphDocumentSaveToastNotification,
} from "./graph-document-save-notifications";

const messages = {
  noticeSaveFailedTitle: "Topology save failed",
  noticeSaveSuccessDescription: "Draft cleared.",
  noticeSaveSuccessTitle: "Topology saved",
  noticeStaleDescription:
    "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
  noticeStaleTitle: "A newer factory definition is available",
};

describe("resolveGraphDocumentSaveToastNotification", () => {
  it("returns success toast when scoped save succeeded and the draft is clean", () => {
    expect(
      resolveGraphDocumentSaveToastNotification({
        documentSave: { status: "success" },
        hasDraftChanges: false,
        messages,
        saveMutationError: null,
      }),
    ).toEqual({
      description: messages.noticeSaveSuccessDescription,
      key: "success",
      kind: "success",
      title: messages.noticeSaveSuccessTitle,
    });
  });

  it("suppresses success toast while draft changes remain", () => {
    expect(
      resolveGraphDocumentSaveToastNotification({
        documentSave: { status: "success" },
        hasDraftChanges: true,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
  });

  it("returns warning toast for stale version save failures with supporting detail", () => {
    const saveMutationError = new CurrentFactoryDefinitionError(
      "The factory definition changed on the server.",
      {
        code: "STALE_FACTORY_VERSION",
        targets: [],
      },
    );

    expect(
      resolveGraphDocumentSaveToastNotification({
        documentSave: {
          message: STALE_FACTORY_GRAPH_DRAFT_WARNING,
          status: "warning",
        },
        hasDraftChanges: true,
        messages,
        saveMutationError,
      }),
    ).toEqual({
      description: buildStaleVersionToastDescription(
        saveMutationError.message,
        messages.noticeStaleDescription,
      ),
      key: `warning:${saveMutationError.message}`,
      kind: "warning",
      title: messages.noticeStaleTitle,
    });
  });

  it("does not toast draft-only stale warnings because the inline notice covers them", () => {
    expect(
      resolveGraphDocumentSaveToastNotification({
        documentSave: {
          message: STALE_FACTORY_GRAPH_DRAFT_WARNING,
          status: "warning",
        },
        hasDraftChanges: true,
        messages,
        saveMutationError: null,
      }),
    ).toBeNull();
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
      resolveGraphDocumentSaveToastNotification({
        documentSave: {
          errorMessage: saveMutationError.message,
          status: "error",
        },
        hasDraftChanges: true,
        messages,
        saveMutationError,
      }),
    ).toEqual({
      description: buildGraphSaveErrorToastDescription(
        saveMutationError.message,
        targets,
      ),
      key: `error:${saveMutationError.message}:${buildGraphSaveErrorToastDescription(
        saveMutationError.message,
        targets,
      )}`,
      kind: "error",
      title: messages.noticeSaveFailedTitle,
    });
  });
});

describe("formatGraphSaveValidationTargetSummary", () => {
  it("returns null when there are no targets", () => {
    expect(formatGraphSaveValidationTargetSummary()).toBeNull();
    expect(formatGraphSaveValidationTargetSummary([])).toBeNull();
  });

  it("deduplicates repeated target labels", () => {
    const target = workerFieldValidationTarget("prompt");
    expect(
      formatGraphSaveValidationTargetSummary([target, target]),
    ).toBe("Affected: WORKER prompt (DEFINITION)");
  });
});
