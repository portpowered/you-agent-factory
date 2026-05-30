import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import { workstationRequest } from "../../base/components/detail-card-test-helpers";
import {
  SelectedWorkDispatchHistorySection,
  WorkstationRequestDetailCard,
} from "./index";

describe("dispatch-selection/public detail components", () => {
  it("renders workstation request detail imported from the public barrel", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-public-smoke", {
          prompt: "Smoke test prompt from public import",
        })}
      />,
    );

    expect(
      screen.getByRole("article", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText("dispatch-public-smoke")).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();
  });

  it("renders dispatch history empty state imported from the public barrel", () => {
    render(
      <CurrentSelectionLocaleProvider>
        <SelectedWorkDispatchHistorySection
          fallbackProviderSessions={[]}
          requests={[]}
          selectedWorkID="work-public-smoke"
          traceTargetId="trace-public-smoke"
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
    expect(
      screen.getByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeTruthy();
  });
});
