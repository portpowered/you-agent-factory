import { act, renderHook, waitFor } from "@testing-library/react";

import { useSelectedProviderSessionState } from "./useSelectedProviderSessionState";
import type { CurrentSelectionState } from "./useCurrentSelection";
import type { DashboardWorkstationRequest } from "../../../api/dashboard/types";

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    failedWorkItems: [],
    openTerminalWorkDetail: () => undefined,
    redoSelection: () => undefined,
    selectedNode: null,
    selectedNodeActiveExecutions: [],
    selectedNodeProviderSessions: [],
    selectedNodeWorkstationRequests: [],
    selectedStateCurrentWorkItems: [],
    selectedStatePlace: null,
    selectedStateTerminalHistoryWorkItems: [],
    selectedStateTokenCount: 0,
    selectedWorkDispatchAttempts: [],
    selectedWorkID: null,
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkWorkstationRequests: [],
    selectedWorkstationRequest: null,
    selection: null,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkByID: () => undefined,
    selectWorkItem: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildRequestWithInferenceSession(
  dispatchID: string,
  sessionID: string,
): DashboardWorkstationRequest {
  return {
    dispatch_id: dispatchID,
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [
      {
        attempt: 1,
        dispatch_id: dispatchID,
        outcome: "SUCCEEDED",
        provider_session: {
          id: sessionID,
          kind: "session_id",
          provider: "codex",
        },
      },
    ],
    prompt: `Prompt for ${sessionID}`,
    responded_request_count: 1,
    transition_id: "transition-review",
    work_items: [],
    workstation_name: "Review",
    workstation_node_id: "workstation-review",
  };
}

describe("useSelectedProviderSessionState", () => {
  it("tracks provider-session selection from work-item inference history", () => {
    const request = buildRequestWithInferenceSession(
      "dispatch-review-active",
      "sess-inference-only",
    );
    const currentSelection = buildCurrentSelection({
      selectedWorkRequestHistory: [request],
      selection: {
        dispatchId: "dispatch-review-active",
        execution: {
          dispatch_id: "dispatch-review-active",
          start_time: "2026-05-22T03:00:00Z",
          status: "RUNNING",
          workstation_node_id: "workstation-review",
          work_items: [],
        },
        kind: "work-item",
        nodeId: "workstation-review",
        workItem: {
          display_name: "Review Story",
          trace_id: "trace-review-story",
          work_id: "work-review-story",
          work_type_id: "story",
        },
      },
    });
    const { result } = renderHook(() =>
      useSelectedProviderSessionState(currentSelection),
    );

    act(() => {
      result.current.setSelectedProviderSession({
        dispatchID: "dispatch-review-active",
        id: "sess-inference-only",
        kind: "session_id",
        provider: "codex",
      });
    });

    expect(result.current.selectedProviderSession?.id).toBe(
      "sess-inference-only",
    );
    expect(result.current.selectedProviderSessionKey).toContain(
      "sess-inference-only",
    );
  });

  it("clears the selected provider session when the active selection no longer exposes it", async () => {
    const firstRequest = buildRequestWithInferenceSession(
      "dispatch-review-active",
      "sess-visible",
    );
    const secondRequest = buildRequestWithInferenceSession(
      "dispatch-review-active",
      "sess-next",
    );
    const { result, rerender } = renderHook(
      (currentSelection: CurrentSelectionState) =>
        useSelectedProviderSessionState(currentSelection),
      {
        initialProps: buildCurrentSelection({
          selectedWorkRequestHistory: [firstRequest],
          selection: {
            dispatchId: "dispatch-review-active",
            execution: {
              dispatch_id: "dispatch-review-active",
              start_time: "2026-05-22T03:00:00Z",
              status: "RUNNING",
              workstation_node_id: "workstation-review",
              work_items: [],
            },
            kind: "work-item",
            nodeId: "workstation-review",
            workItem: {
              display_name: "Review Story",
              trace_id: "trace-review-story",
              work_id: "work-review-story",
              work_type_id: "story",
            },
          },
        }),
      },
    );

    act(() => {
      result.current.setSelectedProviderSession({
        dispatchID: "dispatch-review-active",
        id: "sess-visible",
        kind: "session_id",
        provider: "codex",
      });
    });

    expect(result.current.selectedProviderSession?.id).toBe("sess-visible");

    rerender(
      buildCurrentSelection({
        selectedWorkRequestHistory: [secondRequest],
        selection: {
          dispatchId: "dispatch-review-active",
          execution: {
            dispatch_id: "dispatch-review-active",
            start_time: "2026-05-22T03:00:00Z",
            status: "RUNNING",
            workstation_node_id: "workstation-review",
            work_items: [],
          },
          kind: "work-item",
          nodeId: "workstation-review",
          workItem: {
            display_name: "Review Story",
            trace_id: "trace-review-story",
            work_id: "work-review-story",
            work_type_id: "story",
          },
        },
      }),
    );

    await waitFor(() => {
      expect(result.current.selectedProviderSession).toBeNull();
      expect(result.current.selectedProviderSessionKey).toBeNull();
    });
  });
});
