// @component-test-runner vitest: components package declarations contain relative imports Bun cannot execute.
import "../../../../testing/vitest-dom-capabilities.setup";

import "@testing-library/jest-dom/vitest";
import "./test-support/react-flow-current-activity-card-component.mocks";

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import {
  type ImportFactoryValue,
  SessionFactoryAPIError,
} from "../../../../api/session-factory";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import type { ReadFactoryImportFile } from "../../../import/hooks/use-factory-png-drop";
import { createFactoryImportConfirmInput } from "../../../import/lib/factory-import-confirm-input.test-helpers";
import type { FactoryImportConfirmInput } from "../../../import/lib/factory-import-save-choice";
import type { FactoryPngImportValue } from "../../../import/lib/factory-png-import";
import {
  createFactoryImportValue,
  createFileDropTransfer,
  createImportController,
  PADDING_CLASS_PATTERN,
  registerCurrentActivityCardTestLifecycle,
  renderCurrentActivity,
} from "./test-support/react-flow-current-activity-card-component.harness";

describe("ReactFlowCurrentActivityCard import flows", () => {
  registerCurrentActivityCardTestLifecycle();

  it("scopes file drag-over and drop handling to the graph viewport and opens a preview", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });
    const editModeButton = screen.getByRole("button", { name: "Edit mode" });

    fireEvent.dragOver(editModeButton, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
    expect(readFactoryImportFile).not.toHaveBeenCalled();

    fireEvent.dragOver(viewport, createFileDropTransfer([file]));

    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "drag-active",
    );
    expect(screen.getByText("Import factory PNG")).toBeTruthy();
    expect(
      screen.getByText(
        "Drop a you-agent-factory PNG onto this graph to start import.",
      ),
    ).toBeTruthy();

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    await waitFor(() => {
      expect(readFactoryImportFile).toHaveBeenCalledWith(file);
    });
    await waitFor(() => {
      expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
    });
    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(previewDialog.textContent).toContain(
      "Review the dropped factory before confirming import.",
    );
    expect(
      within(previewDialog)
        .getByRole("img", { name: "Dropped Factory preview" })
        .getAttribute("src"),
    ).toBe("blob:factory-preview");
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Cancel import" }),
    );

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();
  });

  it("closes the factory import preview from the shared dialog close control", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });
    const closeButton = within(previewDialog).getByRole("button", {
      name: "Close import preview",
    });

    fireEvent.click(closeButton);

    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
  });

  it("does not render the import preview inside the graph card when a dashboard controller owns it", () => {
    renderCurrentActivity({
      importController: createImportController({
        importPreviewState: {
          file: new File(["png"], "factory-import.png", { type: "image/png" }),
          status: "ready",
          value: createFactoryImportValue(),
        },
      }),
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(
      screen.getByRole("region", { name: "Work graph viewport" }),
    ).toBeTruthy();
  });

  it("preserves the lean outer card shell while keeping the current activity region semantics", () => {
    renderCurrentActivity({
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const card = screen.getByLabelText("Current activity");
    const viewport = screen.getByRole("region", {
      name: "Work graph viewport",
    });

    expect(card?.className).toContain("relative");
    expect(card?.className).toContain("flex");
    expect(card?.className).toContain("h-full");
    expect(card?.className).toContain("max-h-full");
    expect(card?.className).toContain("min-h-0");
    expect(card?.className).toContain("overflow-hidden");
    expect((card as HTMLElement | null)?.style.height).toBe("100%");
    expect((card as HTMLElement | null)?.style.maxHeight).toBe("100%");
    expect((card as HTMLElement | null)?.style.overflow).toBe("hidden");
    expect(card?.className).not.toMatch(PADDING_CLASS_PATTERN);
    expect(
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(screen.getByText("Observe")).toBeTruthy();
    expect(viewport.className).toContain("relative");
    expect(viewport.className).toContain("min-h-0");
    expect(viewport.className).toContain("overflow-hidden");
    expect(viewport.className).not.toMatch(PADDING_CLASS_PATTERN);
    expect(
      within(viewport).getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();
  });

  it("renders a clear local alert when dropped PNG validation fails", async () => {
    const file = new File(["png"], "invalid-factory.png", {
      type: "image/png",
    });
    const onFactoryImportReady =
      vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        error: {
          code: "PNG_METADATA_MISSING",
          message:
            "The selected PNG does not contain you-agent-factory factory metadata.",
        },
        ok: false,
      });

    renderCurrentActivity({
      onFactoryImportReady,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const alert = await screen.findByRole("alert");

    expect(alert.textContent).toContain("Factory import failed");
    expect(alert.textContent).toContain("invalid-factory.png");
    expect(alert.textContent).toContain(
      "This PNG does not include the you-agent-factory factory metadata needed for import.",
    );
    expect(onFactoryImportReady).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: "Review factory import" }),
    ).toBeNull();
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "error",
    );
    expect(
      screen.getByRole("region", { name: "Factory topology" }),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(viewport.getAttribute("data-current-activity-drop-state")).toBe(
      "idle",
    );
  });

  it("activates the dropped factory, closes the preview, and requests an active-view refresh", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    let resolveActivation: ((value: ImportFactoryValue) => void) | null = null;
    const activateFactory = vi
      .fn<(input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>>()
      .mockImplementation(
        () =>
          new Promise<ImportFactoryValue>((resolve) => {
            resolveActivation = resolve;
          }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Confirm import" }),
    );

    await waitFor(() => {
      expect(activateFactory).toHaveBeenCalledWith(
        createFactoryImportConfirmInput(importValue, {
          existingFactoryNames: ["dashboard-fixture", "Dropped Factory"],
        }),
      );
    });
    const activateButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Activating factory...",
      },
    );
    const cancelButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Cancel import",
      },
    );
    const closeButton = within(previewDialog).getByRole<HTMLButtonElement>(
      "button",
      {
        name: "Close import preview",
      },
    );

    expect(activateButton.getAttribute("aria-busy")).toBe("true");
    expect(activateButton.disabled).toBe(true);
    expect(cancelButton.disabled).toBe(true);
    expect(closeButton.disabled).toBe(true);

    resolveActivation?.(importValue.factory);

    await waitFor(() => {
      expect(onFactoryActivated).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Review factory import" }),
      ).toBeNull();
    });
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
  });

  it("shows a distinct duplicate-name activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>>()
      .mockRejectedValue(
        new SessionFactoryAPIError("Named factory already exists.", {
          code: "FACTORY_ALREADY_EXISTS",
          status: 409,
        }),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Confirm import" }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(activateFactory).toHaveBeenCalledWith(
      createFactoryImportConfirmInput(importValue, {
        existingFactoryNames: ["dashboard-fixture", "Dropped Factory"],
      }),
    );
    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain(
      "A factory with this name already exists.",
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: "Review factory import" }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });

  it("shows a distinct non-idle activation error without changing the current view", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    const activateFactory = vi
      .fn<(input: FactoryImportConfirmInput) => Promise<ImportFactoryValue>>()
      .mockRejectedValue(
        new SessionFactoryAPIError(
          "Current factory runtime must be idle before activation.",
          {
            code: "FACTORY_NOT_IDLE",
            status: 409,
          },
        ),
      );
    const onFactoryActivated = vi.fn<() => void>();
    const readFactoryImportFile = vi
      .fn<ReadFactoryImportFile>()
      .mockResolvedValue({
        ok: true,
        value: importValue,
      });

    renderCurrentActivity({
      activateFactory,
      onFactoryActivated,
      readFactoryImportFile,
      snapshot: semanticWorkflowDashboardSnapshot,
    });

    const viewport = await screen.findByRole("region", {
      name: "Work graph viewport",
    });

    fireEvent.drop(viewport, createFileDropTransfer([file]));

    const previewDialog = await screen.findByRole("dialog", {
      name: "Review factory import",
    });

    fireEvent.click(
      within(previewDialog).getByRole("button", { name: "Confirm import" }),
    );

    const alert = await within(previewDialog).findByRole("alert");

    expect(activateFactory).toHaveBeenCalledWith(
      createFactoryImportConfirmInput(importValue, {
        existingFactoryNames: ["dashboard-fixture", "Dropped Factory"],
      }),
    );
    expect(alert.textContent).toContain("Activation failed");
    expect(alert.textContent).toContain(
      "The current factory runtime is still active.",
    );
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(
      screen.getByRole("dialog", { name: "Review factory import" }),
    ).toBeTruthy();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });
});
