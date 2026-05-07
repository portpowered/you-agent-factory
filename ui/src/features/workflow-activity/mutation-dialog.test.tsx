import { render, screen, within } from "@testing-library/react";

import { getWorkflowActivityGraphImportMessages } from "./messages/graph-import";
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

  it("renders locale-backed shell labels for supported and unsupported locales", () => {
    const japaneseMessages = getWorkflowActivityGraphImportMessages("ja");
    const englishMessages = getWorkflowActivityGraphImportMessages("en");
    const { rerender } = render(
      <DashboardMutationDialog
        description="Review the dropped factory."
        locale="ja"
        onClose={vi.fn()}
        title="Review factory import"
      >
        <p>Import body</p>
      </DashboardMutationDialog>,
    );

    expect(screen.getByText(japaneseMessages.dialogFlowLabel)).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: japaneseMessages.dialogCloseLabel }),
    ).toHaveLength(2);

    rerender(
      <DashboardMutationDialog
        description="Review the dropped factory."
        locale="fr-CA"
        onClose={vi.fn()}
        title="Review factory import"
      >
        <p>Import body</p>
      </DashboardMutationDialog>,
    );

    expect(screen.getByText(englishMessages.dialogFlowLabel)).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: englishMessages.dialogCloseLabel }),
    ).toHaveLength(2);
  });
});
