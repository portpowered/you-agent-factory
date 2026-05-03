import { render, screen, within } from "@testing-library/react";

import { DashboardMutationDialog } from "./mutation-dialog";

describe("DashboardMutationDialog", () => {
  it("assigns unique accessible title and description ids per dialog instance", () => {
    render(
      <>
        <DashboardMutationDialog description="Export the current factory." title="Export factory">
          <p>Export body</p>
        </DashboardMutationDialog>
        <DashboardMutationDialog description="Review the dropped factory." title="Review factory import">
          <p>Import body</p>
        </DashboardMutationDialog>
      </>,
    );

    const exportDialog = screen.getByRole("dialog", { name: "Export factory" });
    const importDialog = screen.getByRole("dialog", { name: "Review factory import" });

    const exportTitleId = exportDialog.getAttribute("aria-labelledby");
    const importTitleId = importDialog.getAttribute("aria-labelledby");
    const exportDescriptionId = exportDialog.getAttribute("aria-describedby");
    const importDescriptionId = importDialog.getAttribute("aria-describedby");

    expect(exportTitleId).toBeTruthy();
    expect(importTitleId).toBeTruthy();
    expect(exportDescriptionId).toBeTruthy();
    expect(importDescriptionId).toBeTruthy();
    expect(exportTitleId).not.toBe(importTitleId);
    expect(exportDescriptionId).not.toBe(importDescriptionId);
    expect(
      within(exportDialog).getByText("Export factory").getAttribute("id"),
    ).toBe(exportTitleId);
    expect(
      within(importDialog).getByText("Review factory import").getAttribute("id"),
    ).toBe(importTitleId);
    expect(
      within(exportDialog).getByText("Export the current factory.").getAttribute("id"),
    ).toBe(exportDescriptionId);
    expect(
      within(importDialog).getByText("Review the dropped factory.").getAttribute("id"),
    ).toBe(importDescriptionId);
  });
});

