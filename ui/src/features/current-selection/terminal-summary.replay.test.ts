import terminalSummaryRegressionReplayFixtureText from "../../../integration/fixtures/terminal-summary-regression-replay.jsonl?raw";

import { buildFactoryTimelineSnapshot } from "../timeline/state/factoryTimelineStore";
import { parseReplayFixtureEvents } from "../../testing/replay-fixtures";
import { buildTerminalWorkItems } from "./useCurrentSelection.selection-helpers";
import { resolveProjectedWorkstationRequestsByDispatchID } from "./useCurrentSelection.request-helpers";
import { resolveTrackedWorkSelection } from "./useCurrentSelection.selection-helpers";

describe("terminal summary replay regression", () => {
  it("uses replayed terminal dispatch outcomes instead of earlier provider-attempt context", () => {
    const events = parseReplayFixtureEvents(
      terminalSummaryRegressionReplayFixtureText,
    );
    const snapshot = buildFactoryTimelineSnapshot(events, 166);
    const projectedWorkstationRequestsByDispatchID =
      resolveProjectedWorkstationRequestsByDispatchID(snapshot, undefined);

    const failedWorkItems = buildTerminalWorkItems(
      snapshot.runtime.session.failed_work_labels ?? [],
      snapshot.runtime.session.provider_sessions,
      snapshot.runtime.session.failed_work_details_by_work_id,
      projectedWorkstationRequestsByDispatchID,
    );

    expect(
      failedWorkItems.find(
        (item) =>
          item.label === "fix-gocoveragecheck-zero-coverage-report-gap",
      ),
    ).toEqual(
      expect.objectContaining({
        contextText: expect.stringContaining(
          "Accepted at executor-loop-breaker; codex / session_id /",
        ),
      }),
    );

    expect(
      failedWorkItems.find(
        (item) =>
          item.label ===
          "split-functionallong-provider-template-helpers-from-default-support",
      ),
    ).toEqual(
      expect.objectContaining({
        contextText: expect.stringContaining(
          "Failed at setup-workspace; codex / session_id /",
        ),
        failureMessage:
          'execution cancelled: exec: "python3": executable file not found in $PATH',
        failureReason: "worker_error",
      }),
    );

    const fixFailedItem = failedWorkItems.find(
      (item) =>
        item.label === "fix-gocoveragecheck-zero-coverage-report-gap",
    );
    if (!fixFailedItem) {
      throw new Error("expected replay fixture to include fix-gocoveragecheck failed item");
    }

    expect(
      resolveTrackedWorkSelection({
        dispatchID: fixFailedItem.dispatchID,
        snapshot,
        terminalWorkDetail: {
          dispatchID: fixFailedItem.dispatchID,
          label: fixFailedItem.label,
          status: "failed",
          traceWorkID: fixFailedItem.traceWorkID,
          workItem: fixFailedItem.workItem,
        },
        workID: fixFailedItem.traceWorkID,
        workstationRequestsByDispatchID: projectedWorkstationRequestsByDispatchID,
      }),
    ).toEqual({
      dispatchId: "eb246a85-7d42-4aa7-9a61-0b3c4b9b284b",
      kind: "work-item",
      nodeId: "executor-loop-breaker",
      workItem: expect.objectContaining({
        work_id:
          "batch-request-078c9da12cc4d347ec1f6a87d3e2253c-fix-gocoveragecheck-zero-coverage-report-gap",
      }),
    });
  });
});
