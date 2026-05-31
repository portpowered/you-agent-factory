import { render } from "@testing-library/react";
import { toast } from "sonner";

import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../notifications/public";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

function createEditorStub(overrides: Record<string, unknown> = {}) {
  return {
    draftState: { hasChanges: false },
    graphDraftSaveSucceeded: false,
    saveAttemptRevision: 0,
    saveEditableDefinition: {
      error: null,
    },
    ...overrides,
  };
}

describe("CurrentActivityGraphSaveNotifications", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the sonner success hook when a graph draft save completes", () => {
    render(
      <CurrentActivityGraphSaveNotifications
        editor={
          createEditorStub({
            graphDraftSaveSucceeded: true,
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
  });

  it("calls the sonner error hook when a graph draft save fails", () => {
    render(
      <CurrentActivityGraphSaveNotifications
        editor={
          createEditorStub({
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
  });

  it("does not repeat the same save notification across rerenders with the same attempt revision", () => {
    const editor = createEditorStub({
      graphDraftSaveSucceeded: true,
      saveAttemptRevision: 1,
    });
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications editor={editor as never} />,
    );

    rerender(
      <CurrentActivityGraphSaveNotifications editor={editor as never} />,
    );

    expect(toast.success).toHaveBeenCalledTimes(1);
  });

  it("calls toast.error twice for the same message on distinct save attempts", () => {
    const { rerender } = render(
      <CurrentActivityGraphSaveNotifications
        editor={
          createEditorStub({
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
        editor={
          createEditorStub({
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
