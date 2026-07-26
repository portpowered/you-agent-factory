import { render } from "@testing-library/react";
import type { ComponentProps } from "react";
import { toast } from "sonner";

import { CurrentFactoryDefinitionError } from "../../../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../../../testing/factory-validation-target-fixtures";
import { buildGraphSaveErrorToastDescription } from "../../../../factory-graph-editor/lib/document-save/graph-document-save-notifications";
import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../../../notifications/lib/save-notification-delivery-policy";
import { CurrentSelectionSaveNotifications } from "./current-selection-save-notifications";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

const messages = {
  saveFailedAffectedSummary: (labels: string) => `Affected: ${labels}`,
  saveFailedTitle: "Worker save failed",
  saveSuccessDescription:
    "Running factory saved. worker-1 was updated in the running factory definition.",
  saveSuccessTitle: "Worker saved",
  staleVersionDetail:
    "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
};

function renderNotifications(
  overrides: Partial<
    ComponentProps<typeof CurrentSelectionSaveNotifications>
  > = {},
) {
  return render(
    <CurrentSelectionSaveNotifications
      documentSave={{ status: "idle" }}
      entityKind="worker"
      hasDraftChanges={false}
      messages={messages}
      saveAttemptRevision={0}
      saveMutationError={null}
      {...overrides}
    />,
  );
}

describe("CurrentSelectionSaveNotifications toast delivery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the sonner success hook when scoped document save succeeds", () => {
    renderNotifications({
      documentSave: { status: "success" },
      saveAttemptRevision: 1,
    });

    expect(toast.success).toHaveBeenCalledWith("Worker saved", {
      description: messages.saveSuccessDescription,
      duration: GLOBAL_TOAST_DURATION_MS,
    });
    expect(toast.error).not.toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
  });

  it("delivers work type success notifications with entity-specific copy", () => {
    renderNotifications({
      documentSave: { status: "success" },
      entityKind: "work-type",
      messages: {
        ...messages,
        saveSuccessDescription:
          'Running factory saved. "feature" was refreshed to the saved definition.',
        saveSuccessTitle: "Work type saved",
      },
      saveAttemptRevision: 1,
    });

    expect(toast.success).toHaveBeenCalledWith("Work type saved", {
      description:
        'Running factory saved. "feature" was refreshed to the saved definition.',
      duration: GLOBAL_TOAST_DURATION_MS,
    });
  });

  it("calls the sonner error hook when scoped document save fails", () => {
    renderNotifications({
      documentSave: {
        errorMessage: "Network dropped",
        status: "error",
      },
      saveAttemptRevision: 1,
    });

    expect(toast.error).toHaveBeenCalledWith("Worker save failed", {
      description: "Network dropped",
      duration: PERSISTENT_TOAST_DURATION_MS,
    });
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
  });

  it("calls the sonner warning hook for stale version save failures", () => {
    const saveMutationError = new CurrentFactoryDefinitionError(
      "The factory definition changed on the server.",
      {
        code: "STALE_FACTORY_VERSION",
        targets: [],
      },
    );

    renderNotifications({
      documentSave: {
        message: saveMutationError.message,
        status: "warning",
      },
      hasDraftChanges: true,
      saveAttemptRevision: 1,
      saveMutationError,
    });

    expect(toast.warning).toHaveBeenCalledWith(saveMutationError.message, {
      description: messages.staleVersionDetail,
      duration: GLOBAL_TOAST_DURATION_MS,
    });
    expect(toast.error).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("includes validation target summary in error toast descriptions", () => {
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

    renderNotifications({
      documentSave: {
        errorMessage: saveMutationError.message,
        status: "error",
      },
      hasDraftChanges: true,
      saveAttemptRevision: 1,
      saveMutationError,
    });

    expect(toast.error).toHaveBeenCalledWith("Worker save failed", {
      description: buildGraphSaveErrorToastDescription(
        saveMutationError.message,
        targets,
        messages.saveFailedAffectedSummary,
      ),
      duration: PERSISTENT_TOAST_DURATION_MS,
    });
  });
});

describe("CurrentSelectionSaveNotifications dedupe and rendering", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not repeat the same save notification across rerenders with the same attempt revision", () => {
    const props = {
      documentSave: { status: "success" as const },
      saveAttemptRevision: 1,
    };
    const { rerender } = renderNotifications(props);

    rerender(
      <CurrentSelectionSaveNotifications
        documentSave={{ status: "idle" }}
        entityKind="worker"
        hasDraftChanges={false}
        messages={messages}
        saveAttemptRevision={0}
        saveMutationError={null}
        {...props}
      />,
    );

    expect(toast.success).toHaveBeenCalledTimes(1);
  });

  it("calls toast.error twice for the same message on distinct save attempts", () => {
    const errorProps = {
      documentSave: {
        errorMessage: "Network dropped",
        status: "error" as const,
      },
    };
    const { rerender } = renderNotifications({
      ...errorProps,
      saveAttemptRevision: 1,
    });

    expect(toast.error).toHaveBeenCalledTimes(1);

    rerender(
      <CurrentSelectionSaveNotifications
        entityKind="worker"
        hasDraftChanges={false}
        messages={messages}
        saveMutationError={null}
        {...errorProps}
        saveAttemptRevision={2}
      />,
    );

    expect(toast.error).toHaveBeenCalledTimes(2);
  });

  it("renders nothing to the DOM", () => {
    const { container } = renderNotifications({
      documentSave: { status: "success" },
      saveAttemptRevision: 1,
    });

    expect(container.innerHTML).toBe("");
  });
});
