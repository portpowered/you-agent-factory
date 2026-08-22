import { describe, expect, it, mock } from "bun:test";
import { render } from "@testing-library/react";

import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../notifications/lib/save-notification-delivery-policy";
import {
  type CurrentActivityGraphSaveNotificationEffects,
  CurrentActivityGraphSaveNotifications,
} from "./react-flow-current-activity-card-save-notifications";

function createNotificationEffects(): CurrentActivityGraphSaveNotificationEffects {
  return {
    error: mock(() => {}),
    success: mock(() => {}),
    warning: mock(() => {}),
  };
}

function createViewModelStub(overrides: Record<string, unknown> = {}) {
  const merged = {
    documentSave: { status: "idle" },
    draftState: { hasChanges: false },
    saveAttemptRevision: 0,
    saveEditableDefinition: {
      error: null,
    },
    ...overrides,
  };
  const hasTopologyChanges =
    (merged.draftState as { hasChanges?: boolean }).hasChanges ?? false;
  const saveMutation = merged.saveEditableDefinition as {
    error?: unknown;
    isPending?: boolean;
  };

  return {
    ...merged,
    saveControls: {
      attemptRevision: merged.saveAttemptRevision,
      feedback: merged.documentSave,
      ...((merged as { saveControls?: object }).saveControls ?? {}),
    },
    status: {
      hasSharedGraphChanges: hasTopologyChanges,
      hasTopologyChanges,
      isSaving: saveMutation.isPending ?? false,
      saveError: saveMutation.error ?? null,
      ...((merged as { status?: object }).status ?? {}),
    },
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: save notification outcomes remain one focused contract across success, warning, error, and retry delivery.
describe("CurrentActivityGraphSaveNotifications", () => {
  it("delivers the success outcome through the injected notification effect", () => {
    const effects = createNotificationEffects();
    render(
      <CurrentActivityGraphSaveNotifications
        editorController={
          createViewModelStub({
            documentSave: { status: "success" },
            saveAttemptRevision: 1,
          }) as never
        }
        notificationEffects={effects}
      />,
    );

    expect(effects.success).toHaveBeenCalledWith("Topology saved", {
      description:
        "The draft has been cleared and the graph is waiting for the latest factory-change event refresh.",
      duration: GLOBAL_TOAST_DURATION_MS,
    });
    expect(effects.error).not.toHaveBeenCalled();
    expect(effects.warning).not.toHaveBeenCalled();
  });

  it("delivers the error outcome through the injected notification effect", () => {
    const effects = createNotificationEffects();
    render(
      <CurrentActivityGraphSaveNotifications
        editorController={
          createViewModelStub({
            documentSave: {
              errorMessage: "The graph is invalid.",
              status: "error",
            },
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: new Error("The graph is invalid."),
            },
          }) as never
        }
        notificationEffects={effects}
      />,
    );

    expect(effects.error).toHaveBeenCalledWith("Topology save failed", {
      description: "The graph is invalid.",
      duration: PERSISTENT_TOAST_DURATION_MS,
    });
    expect(effects.success).not.toHaveBeenCalled();
    expect(effects.warning).not.toHaveBeenCalled();
  });

  it("delivers the stale-version warning through the injected notification effect", () => {
    const effects = createNotificationEffects();
    const saveMutationError = {
      code: "STALE_FACTORY_VERSION",
      message: "The factory definition changed on the server.",
    };

    render(
      <CurrentActivityGraphSaveNotifications
        editorController={
          createViewModelStub({
            documentSave: {
              message:
                "The factory definition changed while you were editing. Refresh or discard your draft before saving.",
              status: "warning",
            },
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: saveMutationError,
            },
          }) as never
        }
        notificationEffects={effects}
      />,
    );

    expect(effects.warning).toHaveBeenCalledWith(
      "A newer factory definition is available",
      {
        description:
          "The factory definition changed on the server.\n\nRefresh or discard the current draft before saving so you do not overwrite a newer topology version.",
        duration: GLOBAL_TOAST_DURATION_MS,
      },
    );
    expect(effects.error).not.toHaveBeenCalled();
    expect(effects.success).not.toHaveBeenCalled();
  });

  it("does not repeat the same save notification across rerenders with the same attempt revision", () => {
    const effects = createNotificationEffects();
    const viewModel = createViewModelStub({
      documentSave: { status: "success" },
      saveAttemptRevision: 1,
    });
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications
        editorController={viewModel as never}
        notificationEffects={effects}
      />,
    );

    rerender(
      <CurrentActivityGraphSaveNotifications
        editorController={viewModel as never}
        notificationEffects={effects}
      />,
    );

    expect(effects.success).toHaveBeenCalledTimes(1);
  });

  it("delivers the same error twice for distinct save attempts", () => {
    const effects = createNotificationEffects();
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications
        editorController={
          createViewModelStub({
            documentSave: {
              errorMessage: "The graph is invalid.",
              status: "error",
            },
            saveAttemptRevision: 1,
            saveEditableDefinition: {
              error: new Error("The graph is invalid."),
            },
          }) as never
        }
        notificationEffects={effects}
      />,
    );

    expect(effects.error).toHaveBeenCalledTimes(1);

    rerender(
      <CurrentActivityGraphSaveNotifications
        editorController={
          createViewModelStub({
            documentSave: {
              errorMessage: "The graph is invalid.",
              status: "error",
            },
            saveAttemptRevision: 2,
            saveEditableDefinition: {
              error: new Error("The graph is invalid."),
            },
          }) as never
        }
        notificationEffects={effects}
      />,
    );

    expect(effects.error).toHaveBeenCalledTimes(2);
  });
});
