import { render } from "@testing-library/react";
import { toast } from "sonner";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../notifications/lib/save-notification-delivery-policy";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

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

describe("CurrentActivityGraphSaveNotifications", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the sonner success hook when scoped document save succeeds", () => {
    render(
      <CurrentActivityGraphSaveNotifications
        viewModel={
          createViewModelStub({
            documentSave: { status: "success" },
            saveAttemptRevision: 1,
          }) as never
        }
      />,
    );

    expect(toast.success).toHaveBeenCalledWith("Topology saved", {
      description:
        "The draft has been cleared and the graph is waiting for the latest factory-change event refresh.",
      duration: GLOBAL_TOAST_DURATION_MS,
    });
    expect(toast.error).not.toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
  });

  it("calls the sonner error hook when scoped document save fails", () => {
    render(
      <CurrentActivityGraphSaveNotifications
        viewModel={
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
      />,
    );

    expect(toast.error).toHaveBeenCalledWith("Topology save failed", {
      description: "The graph is invalid.",
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
      },
    );

    render(
      <CurrentActivityGraphSaveNotifications
        viewModel={
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
      />,
    );

    expect(toast.warning).toHaveBeenCalledWith(
      "A newer factory definition is available",
      {
        description:
          "The factory definition changed on the server.\n\nRefresh or discard the current draft before saving so you do not overwrite a newer topology version.",
        duration: GLOBAL_TOAST_DURATION_MS,
      },
    );
    expect(toast.error).not.toHaveBeenCalled();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("does not repeat the same save notification across rerenders with the same attempt revision", () => {
    const viewModel = createViewModelStub({
      documentSave: { status: "success" },
      saveAttemptRevision: 1,
    });
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications viewModel={viewModel as never} />,
    );

    rerender(
      <CurrentActivityGraphSaveNotifications viewModel={viewModel as never} />,
    );

    expect(toast.success).toHaveBeenCalledTimes(1);
  });

  it("calls toast.error twice for the same message on distinct save attempts", () => {
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications
        viewModel={
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
      />,
    );

    expect(toast.error).toHaveBeenCalledTimes(1);

    rerender(
      <CurrentActivityGraphSaveNotifications
        viewModel={
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
      />,
    );

    expect(toast.error).toHaveBeenCalledTimes(2);
  });
});
