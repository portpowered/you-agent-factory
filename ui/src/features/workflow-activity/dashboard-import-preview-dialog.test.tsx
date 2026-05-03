import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { NamedFactoryAPIError } from "../../api/named-factory";
import type { CurrentActivityImportController } from "./current-activity-import-controller";
import { DashboardImportPreviewDialog } from "./dashboard-import-preview-dialog";

function createImportController(
  overrides: Partial<CurrentActivityImportController> = {},
): CurrentActivityImportController {
  return {
    activateImport: vi.fn().mockResolvedValue(undefined),
    activationState: { status: "idle" },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" },
    importPreviewState: {
      file: new File(["png"], "factory-import.png", { type: "image/png" }),
      status: "ready",
      value: {
        factory: {
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        previewImageSrc: "blob:factory-preview",
        revokePreviewImageSrc: vi.fn(),
        schemaVersion: "portos.agent-factory.png.v1",
      },
    },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
    ...overrides,
  };
}

describe("DashboardImportPreviewDialog", () => {
  it("renders the import preview from the dashboard-owned controller", async () => {
    const importController = createImportController();

    render(<DashboardImportPreviewDialog importController={importController} />);

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(
      within(previewDialog).getByRole("img", { name: "Dropped Factory preview" }).getAttribute("src"),
    ).toBe("blob:factory-preview");

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Cancel import" }));

    expect(importController.clearActivationError).toHaveBeenCalledTimes(1);
    expect(importController.closeImportPreview).toHaveBeenCalledTimes(1);
  });

  it("blocks close interactions while activation is submitting", async () => {
    const importController = createImportController({
      activationState: { status: "submitting" },
    });

    render(<DashboardImportPreviewDialog importController={importController} />);

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
    const closeButton = within(previewDialog).getByRole("button", {
      name: "Close import preview",
    });
    const cancelButton = within(previewDialog).getByRole("button", { name: "Cancel import" });

    fireEvent.click(closeButton);
    fireEvent.click(cancelButton);

    expect(closeButton.getAttribute("disabled")).not.toBeNull();
    expect(cancelButton.getAttribute("disabled")).not.toBeNull();
    expect(importController.closeImportPreview).not.toHaveBeenCalled();
  });

  it("shows activation failures and delegates confirmation through the controller", async () => {
    const importController = createImportController({
      activationState: {
        error: new NamedFactoryAPIError("Network unreachable", { code: "NETWORK_ERROR" }),
        status: "error",
      },
    });

    render(<DashboardImportPreviewDialog importController={importController} />);

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    expect(previewDialog.textContent).toContain("Activation failed");
    expect(previewDialog.textContent).toContain(
      "The dashboard could not reach the activation API. Try again once the connection is available.",
    );

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    await waitFor(() => {
      expect(importController.activateImport).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({ name: "Dropped Factory" }),
        }),
      );
    });
  });

  it("dismisses the dashboard-owned preview after a successful activation", async () => {
    const activateImport = vi.fn().mockResolvedValue(undefined);

    function ImportPreviewSuccessHarness() {
      const [importPreviewState, setImportPreviewState] =
        useState<CurrentActivityImportController["importPreviewState"]>({
          file: new File(["png"], "factory-import.png", { type: "image/png" }),
          status: "ready",
          value: {
            factory: {
              name: "Dropped Factory",
              workTypes: [],
              workers: [],
              workstations: [],
            },
            previewImageSrc: "blob:factory-preview",
            revokePreviewImageSrc: vi.fn(),
            schemaVersion: "portos.agent-factory.png.v1",
          },
        });

      return (
        <DashboardImportPreviewDialog
          importController={createImportController({
            activateImport: async (value) => {
              await activateImport(value);
              setImportPreviewState({ status: "idle" });
            },
            closeImportPreview: () => {
              setImportPreviewState({ status: "idle" });
            },
            importPreviewState,
          })}
        />
      );
    }

    render(<ImportPreviewSuccessHarness />);

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    await waitFor(() => {
      expect(activateImport).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({ name: "Dropped Factory" }),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
    });
  });

  it("does not render when no preview is ready", () => {
    render(
      <DashboardImportPreviewDialog
        importController={createImportController({
          importPreviewState: { status: "idle" },
        })}
      />,
    );

    expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
  });
});
