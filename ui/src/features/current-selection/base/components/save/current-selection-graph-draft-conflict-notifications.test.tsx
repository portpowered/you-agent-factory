import { render } from "@testing-library/react";
import type { ComponentProps } from "react";
import { toast } from "sonner";

import { GLOBAL_TOAST_DURATION_MS } from "../../../../notifications/lib/save-notification-delivery-policy";
import { getCurrentSelectionGraphDraftConflictMessages } from "../../messages/operational/current-selection-graph-draft-conflict";
import { CurrentSelectionGraphDraftConflictNotifications } from "./current-selection-graph-draft-conflict-notifications";
import { CurrentSelectionSaveNotifications } from "./current-selection-save-notifications";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    warning: vi.fn(),
  },
}));

const conflictMessages = getCurrentSelectionGraphDraftConflictMessages("en");

const saveMessages = {
  saveFailedAffectedSummary: (labels: string) => `Affected: ${labels}`,
  saveFailedTitle: "Worker save failed",
  saveSuccessDescription: "Running factory saved.",
  saveSuccessTitle: "Worker saved",
  staleVersionDetail: "Reload the latest running-factory values.",
};

function renderConflictNotifications(
  overrides: Partial<
    ComponentProps<typeof CurrentSelectionGraphDraftConflictNotifications>
  > = {},
) {
  return render(
    <CurrentSelectionGraphDraftConflictNotifications
      documentSave={{ status: "idle" }}
      graphDraftHasPendingChanges={false}
      isTopologyAffectingSave={false}
      saveAttemptRevision={0}
      {...overrides}
    />,
  );
}

describe("CurrentSelectionGraphDraftConflictNotifications toast delivery", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls toast.warning with conflict copy when save conflict applies", () => {
    renderConflictNotifications({
      documentSave: { status: "success" },
      graphDraftHasPendingChanges: true,
      isTopologyAffectingSave: true,
      saveAttemptRevision: 1,
    });

    expect(toast.warning).toHaveBeenCalledWith(
      conflictMessages.graphDraftConflictWarningTitle,
      {
        description: conflictMessages.graphDraftConflictWarningDescription,
        duration: GLOBAL_TOAST_DURATION_MS,
      },
    );
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("does not warn when the graph draft is clean", () => {
    renderConflictNotifications({
      documentSave: { status: "success" },
      graphDraftHasPendingChanges: false,
      isTopologyAffectingSave: true,
      saveAttemptRevision: 1,
    });

    expect(toast.warning).not.toHaveBeenCalled();
  });

  it("does not repeat the same warning across rerenders with the same attempt revision", () => {
    const props = {
      documentSave: { status: "success" as const },
      graphDraftHasPendingChanges: true,
      isTopologyAffectingSave: true,
      saveAttemptRevision: 1,
    };
    const { rerender } = renderConflictNotifications(props);

    rerender(
      <CurrentSelectionGraphDraftConflictNotifications
        locale={undefined}
        {...props}
      />,
    );

    expect(toast.warning).toHaveBeenCalledTimes(1);
  });

  it("calls toast.warning again for a new save attempt revision", () => {
    const sharedProps = {
      documentSave: { status: "success" as const },
      graphDraftHasPendingChanges: true,
      isTopologyAffectingSave: true,
    };
    const { rerender } = renderConflictNotifications({
      ...sharedProps,
      saveAttemptRevision: 1,
    });

    expect(toast.warning).toHaveBeenCalledTimes(1);

    rerender(
      <CurrentSelectionGraphDraftConflictNotifications
        locale={undefined}
        {...sharedProps}
        saveAttemptRevision={2}
      />,
    );

    expect(toast.warning).toHaveBeenCalledTimes(2);
  });

  it("renders nothing to the DOM", () => {
    const { container } = renderConflictNotifications({
      documentSave: { status: "success" },
      graphDraftHasPendingChanges: true,
      isTopologyAffectingSave: true,
      saveAttemptRevision: 1,
    });

    expect(container.innerHTML).toBe("");
  });

  it("can deliver entity success and graph-draft conflict warnings for the same save attempt", () => {
    render(
      <>
        <CurrentSelectionSaveNotifications
          documentSave={{ status: "success" }}
          entityKind="worker"
          hasDraftChanges={false}
          messages={saveMessages}
          saveAttemptRevision={1}
          saveMutationError={null}
        />
        <CurrentSelectionGraphDraftConflictNotifications
          documentSave={{ status: "success" }}
          graphDraftHasPendingChanges={true}
          isTopologyAffectingSave={true}
          saveAttemptRevision={1}
        />
      </>,
    );

    expect(toast.success).toHaveBeenCalledWith("Worker saved", {
      description: saveMessages.saveSuccessDescription,
      duration: GLOBAL_TOAST_DURATION_MS,
    });
    expect(toast.warning).toHaveBeenCalledWith(
      conflictMessages.graphDraftConflictWarningTitle,
      {
        description: conflictMessages.graphDraftConflictWarningDescription,
        duration: GLOBAL_TOAST_DURATION_MS,
      },
    );
  });
});
