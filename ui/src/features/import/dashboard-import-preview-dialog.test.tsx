import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import { NamedFactoryAPIError } from "../../api/named-factory";
import {
  DashboardImportPreviewDialog,
  type DashboardImportPreviewDialogProps,
} from "./dashboard-import-preview-dialog";

function createReadyImportPreviewState(): DashboardImportPreviewDialogProps["importPreviewState"] {
  return {
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
  };
}

function renderDialog(
  overrides: Partial<DashboardImportPreviewDialogProps> = {},
) {
  const onCancel = vi.fn();
  const onConfirm = vi.fn().mockResolvedValue(undefined);

  render(
    <DashboardImportPreviewDialog
      activationState={{ status: "idle" }}
      importPreviewState={createReadyImportPreviewState()}
      onCancel={onCancel}
      onConfirm={onConfirm}
      {...overrides}
    />,
  );

  return { onCancel, onConfirm };
}

describe("DashboardImportPreviewDialog", () => {
  it("renders the extracted dashboard-owned import preview", async () => {
    const { onCancel } = renderDialog();

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(
      within(previewDialog).getByRole("img", { name: "Dropped Factory preview" }).getAttribute("src"),
    ).toBe("blob:factory-preview");

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Cancel import" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("blocks close interactions while activation is submitting", async () => {
    const { onCancel } = renderDialog({
      activationState: { status: "submitting" },
    });

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
    const closeButton = within(previewDialog).getByRole("button", {
      name: "Close import preview",
    });
    const cancelButton = within(previewDialog).getByRole("button", { name: "Cancel import" });

    fireEvent.click(closeButton);
    fireEvent.click(cancelButton);

    expect(closeButton.getAttribute("disabled")).not.toBeNull();
    expect(cancelButton.getAttribute("disabled")).not.toBeNull();
    expect(onCancel).not.toHaveBeenCalled();
  });

  it("clears activation error and closes the preview when cancel delegates through the dashboard seam", async () => {
    function ImportPreviewCancelHarness() {
      const [activationState, setActivationState] =
        useState<DashboardImportPreviewDialogProps["activationState"]>({
          error: new NamedFactoryAPIError("Network unreachable", { code: "NETWORK_ERROR" }),
          status: "error",
        });
      const [importPreviewState, setImportPreviewState] =
        useState<DashboardImportPreviewDialogProps["importPreviewState"]>(
          createReadyImportPreviewState(),
        );

      return (
        <DashboardImportPreviewDialog
          activationState={activationState}
          importPreviewState={importPreviewState}
          onCancel={() => {
            setActivationState({ status: "idle" });
            setImportPreviewState({ status: "idle" });
          }}
          onConfirm={vi.fn()}
        />
      );
    }

    render(<ImportPreviewCancelHarness />);

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });
    expect(previewDialog.textContent).toContain("Activation failed");

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Cancel import" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
    });
  });

  it("shows activation failures and delegates confirmation with the ready preview payload", async () => {
    const { onConfirm } = renderDialog({
      activationState: {
        error: new NamedFactoryAPIError("Network unreachable", { code: "NETWORK_ERROR" }),
        status: "error",
      },
    });

    const previewDialog = await screen.findByRole("dialog", { name: "Review factory import" });

    expect(previewDialog.textContent).toContain("Activation failed");
    expect(previewDialog.textContent).toContain(
      "The dashboard could not reach the activation API. Try again once the connection is available.",
    );

    fireEvent.click(within(previewDialog).getByRole("button", { name: "Activate factory" }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith(
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
        useState<DashboardImportPreviewDialogProps["importPreviewState"]>(
          createReadyImportPreviewState(),
        );

      return (
        <DashboardImportPreviewDialog
          activationState={{ status: "idle" }}
          importPreviewState={importPreviewState}
          onCancel={() => {
            setImportPreviewState({ status: "idle" });
          }}
          onConfirm={async (value) => {
            await activateImport(value);
            setImportPreviewState({ status: "idle" });
          }}
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
        activationState={{ status: "idle" }}
        importPreviewState={{ status: "idle" }}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.queryByRole("dialog", { name: "Review factory import" })).toBeNull();
  });
});
