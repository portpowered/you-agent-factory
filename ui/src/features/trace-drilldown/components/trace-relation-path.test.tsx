// @component-test-runner vitest: imports workspace graph packages that Bun resolves through declaration files.
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { DashboardTraceDispatch } from "../../../api/dashboard/types";
import { buildTraceDispatchFactoryGraphFlow } from "../lib/trace-dispatch-factory-graph-flow";
import { traceSelectionKey } from "../lib/trace-selection";
import { TraceRelationPath } from "./trace-relation-path";

function buildDispatch(
  dispatchID: string,
  overrides: Partial<DashboardTraceDispatch> = {},
): DashboardTraceDispatch {
  return {
    dispatch_id: dispatchID,
    duration_millis: 1000,
    end_time: "2026-04-22T18:00:01Z",
    outcome: "ACCEPTED",
    start_time: "2026-04-22T18:00:00Z",
    transition_id: dispatchID,
    workstation_name: dispatchID,
    ...overrides,
  };
}

describe("TraceRelationPath", () => {
  it("renders the same canonical relation ids and complete endpoint identities as the dispatch graph", () => {
    const flow = buildTraceDispatchFactoryGraphFlow([
      buildDispatch("dispatch-plan", {
        attempt: 1,
        output_items: [{ work_id: "work-shared" }],
      }),
      buildDispatch("dispatch-retry", {
        attempt: 2,
        input_items: [{ work_id: "work-shared" }],
      }),
    ]);
    const onSelectTraceSelection = vi.fn();

    render(
      <TraceRelationPath
        entries={flow.relations}
        onSelectTraceSelection={onSelectTraceSelection}
      />,
    );

    const relationPath = screen.getByRole("region", {
      name: "Textual relation path",
    });
    expect(
      [
        ...relationPath.querySelectorAll<HTMLElement>(
          "[data-trace-relation-entry]",
        ),
      ].map((entry) => entry.getAttribute("data-trace-relation-id")),
    ).toEqual(flow.relations.map((relation) => relation.id));

    const targetSelection = flow.relations[0]?.target.selectionIdentities[0];
    if (!targetSelection) {
      throw new Error("Expected a target selection identity.");
    }

    const targetButton = within(relationPath).getByRole("button", {
      name: "dispatch-retry · Work work-shared · attempt 2",
    });
    expect(targetButton.getAttribute("data-trace-selection-key")).toBe(
      traceSelectionKey(targetSelection),
    );
    fireEvent.click(targetButton);
    expect(onSelectTraceSelection).toHaveBeenCalledWith(targetSelection);
  });

  it("keeps an accessible empty fallback when no relation entries exist", () => {
    render(<TraceRelationPath entries={[]} />);

    const relationPath = screen.getByRole("region", {
      name: "Textual relation path",
    });
    expect(within(relationPath).getByRole("status").textContent).toBe(
      "No recorded relations.",
    );
  });
});
