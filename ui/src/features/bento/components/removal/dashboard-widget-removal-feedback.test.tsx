import "../../../../testing/vitest-dom-capabilities.setup";
import "@testing-library/jest-dom/vitest";

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { AgentBentoLayoutItem } from "../../components/agent-bento";
import type {
  DashboardWidgetRemovalCandidate,
  DashboardWidgetUndoState,
} from "../../hooks/removal/useDashboardWidgetRemoval";
import { DashboardWidgetRemovalFeedback } from "./dashboard-widget-removal-feedback";

const removedItem: AgentBentoLayoutItem = {
  h: 2,
  id: "work-totals::primary",
  minH: 1,
  minW: 1,
  w: 4,
  widgetType: "work-totals",
  x: 0,
  y: 0,
};

const candidate: DashboardWidgetRemovalCandidate = {
  originalIndex: 0,
  removedItem,
  triggerElement: null,
  widgetTitle: "Work totals",
};

function createUndoState(
  overrides: Partial<DashboardWidgetUndoState> = {},
): DashboardWidgetUndoState {
  return {
    ...candidate,
    status: "available",
    storageWriteFailed: false,
    ...overrides,
  };
}

function renderFeedback(
  overrides: Partial<
    React.ComponentProps<typeof DashboardWidgetRemovalFeedback>
  > = {},
) {
  return render(
    <DashboardWidgetRemovalFeedback
      onCancelRemoval={vi.fn()}
      onConfirmRemoval={vi.fn()}
      onDismissUndo={vi.fn()}
      onDialogCloseAutoFocus={vi.fn()}
      onDialogOpenChange={vi.fn()}
      onUndoRemoval={vi.fn()}
      pendingRemoval={candidate}
      undoState={null}
      {...overrides}
    />,
  );
}

describe("DashboardWidgetRemovalFeedback", () => {
  it("names a dirty card in an accessible confirmation dialog and routes cancel/confirm actions", () => {
    const onCancelRemoval = vi.fn();
    const onConfirmRemoval = vi.fn();

    renderFeedback({ onCancelRemoval, onConfirmRemoval });

    expect(screen.getByRole("dialog")).toHaveTextContent(
      "Work totals has unsaved changes",
    );
    expect(screen.getByRole("button", { name: "Keep widget" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove widget" })).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Keep widget" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove widget" }));

    expect(onCancelRemoval).toHaveBeenCalledTimes(1);
    expect(onConfirmRemoval).toHaveBeenCalledTimes(1);
  });

  it("closes the confirmation dialog through Escape", () => {
    const onDialogOpenChange = vi.fn();

    renderFeedback({ onDialogOpenChange });

    fireEvent.keyDown(screen.getByRole("dialog"), {
      code: "Escape",
      key: "Escape",
    });

    expect(onDialogOpenChange).toHaveBeenCalledWith(false);
  });

  it("exposes undo as a status action and reports failed restoration as an alert", () => {
    const onUndoRemoval = vi.fn();
    const onDismissUndo = vi.fn();

    renderFeedback({
      onDismissUndo,
      onUndoRemoval,
      pendingRemoval: null,
      undoState: createUndoState(),
    });

    expect(screen.getByRole("status")).toHaveTextContent(
      "Work totals was removed.",
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Undo removing Work totals" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(onUndoRemoval).toHaveBeenCalledTimes(1);
    expect(onDismissUndo).toHaveBeenCalledTimes(1);

    renderFeedback({
      pendingRemoval: null,
      undoState: createUndoState({ status: "failed-to-persist" }),
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Work totals was restored here, but the layout could not be saved.",
    );
  });
});
