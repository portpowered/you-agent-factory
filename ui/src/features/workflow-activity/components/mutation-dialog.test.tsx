import { render, screen, within } from "@testing-library/react";

import { getWorkflowActivityGraphImportMessages } from "../messages/graph-import";
import { DashboardMessagePanel, DashboardMutationDialog } from "./mutation-dialog";

describe("DashboardMutationDialog", () => {
  it("assigns unique accessible title and description ids per dialog instance", () => {
    render(
      <>
        <DashboardMutationDialog
          description="Export the current factory."
          title="Export factory"
        >
          <p>Export body</p>
        </DashboardMutationDialog>
        <DashboardMutationDialog
          description="Review the dropped factory."
          title="Review factory import"
        >
          <p>Import body</p>
        </DashboardMutationDialog>
      </>,
    );

    const exportDialog = screen.getByRole("dialog", { name: "Export factory" });
    const importDialog = screen.getByRole("dialog", {
      name: "Review factory import",
    });

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
      within(importDialog)
        .getByText("Review factory import")
        .getAttribute("id"),
    ).toBe(importTitleId);
    expect(
      within(exportDialog)
        .getByText("Export the current factory.")
        .getAttribute("id"),
    ).toBe(exportDescriptionId);
    expect(
      within(importDialog)
        .getByText("Review the dropped factory.")
        .getAttribute("id"),
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
      screen.getAllByRole("button", {
        name: japaneseMessages.dialogCloseLabel,
      }),
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
    const importDialog = screen.getByRole("dialog", {
      name: "Review factory import",
    });
    expect(importDialog.className).toContain("border-outline");
    expect(importDialog.className).toContain("bg-surface-container-high");
  });

  it("retains panel surface classes on the dialog section", () => {
    render(
      <DashboardMutationDialog
        description="Export the current factory."
        title="Export factory"
      >
        <p>Export body</p>
      </DashboardMutationDialog>,
    );

    const dialog = screen.getByRole("dialog", { name: "Export factory" });
    expect(dialog.className).toContain("border-outline");
    expect(dialog.className).toContain("bg-surface-container-high");
    expect(dialog.className).toContain("shadow-af-panel");
  });

  it("renders header close button with locale-backed label and focus or hover styling", () => {
    const messages = getWorkflowActivityGraphImportMessages("en");
    render(
      <DashboardMutationDialog
        description="Review the dropped factory."
        onClose={vi.fn()}
        showCloseButton
        title="Review factory import"
      >
        <p>Import body</p>
      </DashboardMutationDialog>,
    );

    const dialog = screen.getByRole("dialog", {
      name: "Review factory import",
    });
    const headerClose = within(dialog).getByRole("button", {
      name: messages.dialogCloseLabel,
    });

    expect(headerClose.className).toContain("rounded-full");
    expect(headerClose.className).toContain("hover:bg-af-overlay");
    expect(headerClose.className).toContain("focus-visible:ring-2");
    expect(headerClose.className).toContain("focus-visible:ring-af-focus-ring");
  });

  it("renders footer actions in an end-aligned flex wrapper", () => {
    render(
      <DashboardMutationDialog
        description="Review the dropped factory."
        footer={<button type="button">Confirm import</button>}
        title="Review factory import"
      >
        <p>Import body</p>
      </DashboardMutationDialog>,
    );

    const footerAction = screen.getByRole("button", { name: "Confirm import" });
    const footerWrapper = footerAction.parentElement;

    expect(footerWrapper?.className).toContain("flex");
    expect(footerWrapper?.className).toContain("justify-end");
    expect(footerWrapper?.className).toContain("gap-3");
  });
});

describe("DashboardMessagePanel", () => {
  it("renders neutral and error message panels through shared alert tones", () => {
    render(
      <>
        <DashboardMessagePanel role="status" title="Ready">
          Import preview ready.
        </DashboardMessagePanel>
        <DashboardMessagePanel role="alert" title="Failed" tone="error">
          Import failed.
        </DashboardMessagePanel>
      </>,
    );

    expect(screen.getByRole("status").className).toContain(
      "bg-surface-container-low",
    );
    expect(screen.getByRole("alert").className).toContain(
      "bg-error-container",
    );
  });
});
