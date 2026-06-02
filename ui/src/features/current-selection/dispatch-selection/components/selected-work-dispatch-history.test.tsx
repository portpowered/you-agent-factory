import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import { workstationRequest } from "../../base/components/detail-card-test-helpers";
import { SelectedWorkDispatchHistorySection } from "./selected-work-dispatch-history";

describe("SelectedWorkDispatchHistorySection", () => {
  it("uses the unified operations heading when operation history is provided", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          fallbackProviderSessions={[]}
          operationHistory={[]}
          requests={[]}
          selectedWorkID="work-ops"
          traceTargetId="trace-ops"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByRole("heading", { name: "Work operations" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "No move or workstation operations have been recorded yet for this work item.",
      ),
    ).toBeTruthy();
  });

  it("keeps the workstation-only heading when operation history is omitted", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          fallbackProviderSessions={[]}
          requests={[
            workstationRequest("dispatch-legacy", {
              workstation_name: "Review",
            }),
          ]}
          selectedWorkID="work-legacy"
          traceTargetId="trace-legacy"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(
      screen.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
  });
});
