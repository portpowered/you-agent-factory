import { renderHook } from "@testing-library/react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { useSelectionSynchronization } from "./useCurrentSelection.synchronization";

describe("useSelectionSynchronization", () => {
  it("resets selection history when the dashboard snapshot is unavailable", () => {
    const resetSelectionHistory = vi.fn();
    const reconcilePresent = vi.fn();

    renderHook(() =>
      useSelectionSynchronization({
        pendingFactoryDefinition: undefined,
        projectedWorkstationRequestsByDispatchID: undefined,
        reconcilePresent,
        resetSelectionHistory,
        snapshot: null,
        topologyFactory: undefined,
      }),
    );

    expect(resetSelectionHistory).toHaveBeenCalledTimes(1);
    expect(reconcilePresent).not.toHaveBeenCalled();
  });

  it("reconciles selection against the saved and pending factory documents", () => {
    const resetSelectionHistory = vi.fn();
    const reconcilePresent = vi.fn((reconcile) =>
      reconcile({ selection, terminalWorkDetail: null }),
    );
    const snapshot = {
      factory: {
        name: "Current Factory",
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "# Guide" },
              targetPath: "factory/docs/guide.md",
              type: "DOC",
            },
          ],
        },
      },
    } as DashboardSnapshot;
    const selection = {
      kind: "doc" as const,
      targetPath: "factory/docs/guide.md",
    };

    renderHook(() =>
      useSelectionSynchronization({
        pendingFactoryDefinition: snapshot.factory,
        projectedWorkstationRequestsByDispatchID: {},
        reconcilePresent,
        resetSelectionHistory,
        snapshot,
        topologyFactory: snapshot.factory,
      }),
    );

    expect(resetSelectionHistory).not.toHaveBeenCalled();
    expect(reconcilePresent).toHaveBeenCalledTimes(1);
    expect(reconcilePresent.mock.results[0]?.value).toEqual({
      selection,
      terminalWorkDetail: null,
    });
  });
});
