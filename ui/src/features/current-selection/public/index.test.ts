import { describe, expect, it } from "bun:test";

import * as currentSelectionPublic from "./index";
import type {
  DashboardSelection,
  DashboardWorkItemSelection,
  DashboardWorkstationRequestSelection,
  StatePositionWorkItem,
} from "./index";

describe("current-selection public barrel", () => {
  it("keeps the public runtime surface focused on the widget and hooks", () => {
    expect(Object.keys(currentSelectionPublic).sort()).toEqual([
      "CurrentSelectionWidget",
      "useCurrentSelection",
      "useCurrentSelectionDetails",
      "useSelectedProviderSessionState",
    ]);
    expect("TerminalWorkDetail" in currentSelectionPublic).toBe(false);
  });

  it("keeps the current-selection contract types available", () => {
    const workItem: StatePositionWorkItem = {
      work_id: "work-1",
    };
    const workItemSelection: DashboardWorkItemSelection = {
      kind: "work-item",
      nodeId: "node-1",
      workItem,
    };
    const workstationRequestSelection: DashboardWorkstationRequestSelection = {
      dispatchId: "dispatch-1",
      kind: "workstation-request",
      nodeId: "node-1",
      request: {
        created_at: "2026-01-01T00:00:00Z",
        dispatch_id: "dispatch-1",
        request: {
          name: "Request 1",
          request_id: "request-1",
          work_items: [],
        },
        workstation_name: "review",
      },
    };
    const selection: DashboardSelection = workItemSelection;

    expect(selection.kind).toBe("work-item");
    expect(workstationRequestSelection.request.dispatch_id).toBe("dispatch-1");
    expect(workItem.work_id).toBe("work-1");
  });
});
