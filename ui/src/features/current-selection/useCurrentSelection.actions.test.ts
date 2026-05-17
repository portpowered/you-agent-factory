import { beforeEach, describe, expect, it, vi } from "vitest";

import type {
  DashboardPlaceRef,
  DashboardSelection,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type { TerminalWorkItem } from "../terminal-work/terminal-work-card";
import { useCurrentSelectionActions } from "./useCurrentSelection.actions";

const helperMocks = vi.hoisted(() => ({
  findTerminalWorkItem: vi.fn(),
  inferStateWorkTerminalStatus: vi.fn(),
  placeNodeID: vi.fn(),
  resolveTrackedWorkSelection: vi.fn(),
}));

vi.mock("./useCurrentSelection.helpers", () => helperMocks);

describe("useCurrentSelectionActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("opens terminal work detail with a workstation-request preference and falls back to the current selection when unresolved", () => {
    const commitSelectionState = vi.fn();
    const currentSelection: DashboardSelection = {
      kind: "node",
      nodeId: "review",
    };
    const workItem: DashboardWorkItemRef = {
      display_name: "Blocked Analysis Story",
      work_id: "work-blocked-analysis",
      work_type_id: "story",
    };
    const terminalItem: TerminalWorkItem = {
      attempts: [],
      dispatchID: "dispatch-blocked-analysis",
      failureMessage: "Provider rate limit exceeded.",
      failureReason: "provider_rate_limit",
      label: "Blocked Analysis Story",
      traceWorkID: workItem.work_id,
      workItem,
    };

    helperMocks.resolveTrackedWorkSelection.mockReturnValueOnce(null);

    const actions = useCurrentSelectionActions({
      commitSelectionState,
      completedWorkItems: [],
      failedWorkItems: [],
      projectedWorkstationRequestsByDispatchID: undefined,
      selection: currentSelection,
      snapshot: null,
      terminalWorkDetail: null,
    });

    actions.openTerminalWorkDetail("failed", terminalItem);

    expect(helperMocks.resolveTrackedWorkSelection).toHaveBeenCalledWith(
      expect.objectContaining({
        dispatchID: "dispatch-blocked-analysis",
        terminalWorkDetail: expect.objectContaining({
          dispatchID: "dispatch-blocked-analysis",
          failureMessage: "Provider rate limit exceeded.",
          failureReason: "provider_rate_limit",
          label: "Blocked Analysis Story",
          preferWorkstationRequest: true,
          status: "failed",
          traceWorkID: "work-blocked-analysis",
          workItem,
        }),
        workID: "work-blocked-analysis",
      }),
    );
    expect(commitSelectionState).toHaveBeenCalledWith({
      selection: currentSelection,
      terminalWorkDetail: expect.objectContaining({
        preferWorkstationRequest: true,
      }),
    });
  });

  it("reuses terminal detail when selecting the same work id", () => {
    const commitSelectionState = vi.fn();
    const resolvedSelection: DashboardSelection = {
      kind: "node",
      nodeId: "plan",
    };

    helperMocks.resolveTrackedWorkSelection.mockReturnValueOnce(resolvedSelection);

    const terminalWorkDetail = {
      label: "Alpha Story",
      status: "failed" as const,
      traceWorkID: "work-alpha",
    };

    const actions = useCurrentSelectionActions({
      commitSelectionState,
      completedWorkItems: [],
      failedWorkItems: [],
      projectedWorkstationRequestsByDispatchID: undefined,
      selection: null,
      snapshot: null,
      terminalWorkDetail,
    });

    actions.selectWorkByID("work-alpha");

    expect(commitSelectionState).toHaveBeenCalledWith({
      selection: resolvedSelection,
      terminalWorkDetail,
    });
  });

  it("falls back to derived terminal detail for state work when no terminal row exists", () => {
    const commitSelectionState = vi.fn();
    const failedPlace: DashboardPlaceRef = {
      kind: "work_state",
      place_id: "story:blocked",
      state_category: "FAILED",
    };
    const workItem: DashboardWorkItemRef = {
      display_name: "Blocked Analysis Story",
      work_id: "work-blocked-analysis",
      work_type_id: "story",
    };

    helperMocks.placeNodeID.mockReturnValueOnce("review");
    helperMocks.resolveTrackedWorkSelection.mockReturnValueOnce(null);
    helperMocks.inferStateWorkTerminalStatus.mockReturnValueOnce("failed");
    helperMocks.findTerminalWorkItem.mockReturnValueOnce(undefined);

    const actions = useCurrentSelectionActions({
      commitSelectionState,
      completedWorkItems: [],
      failedWorkItems: [],
      projectedWorkstationRequestsByDispatchID: undefined,
      selection: null,
      snapshot: {
        runtime: {
          session: {
            failed_work_details_by_work_id: {
              [workItem.work_id]: {
                dispatch_id: "dispatch-blocked-analysis",
                failure_message: "Provider rate limit exceeded while generating the analysis.",
                failure_reason: "provider_rate_limit",
                transition_id: "review",
                work_item: workItem,
                workstation_name: "Review",
              },
            },
          },
        },
      } as never,
      terminalWorkDetail: null,
    });

    actions.selectStateWorkItem(failedPlace, workItem);

    expect(commitSelectionState).toHaveBeenCalledWith({
      selection: null,
      terminalWorkDetail: {
        failureMessage: "Provider rate limit exceeded while generating the analysis.",
        failureReason: "provider_rate_limit",
        label: "Blocked Analysis Story",
        status: "failed",
        traceWorkID: "work-blocked-analysis",
        workItem,
      },
    });
  });
});
