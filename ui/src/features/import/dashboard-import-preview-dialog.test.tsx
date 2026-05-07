import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { axe } from "jest-axe";

import { NamedFactoryAPIError } from "../../api/named-factory";
import {
  DashboardImportPreviewDialog,
  type DashboardImportPreviewDialogProps,
} from "./dashboard-import-preview-dialog";
import { getImportPreviewDialogMessages } from "./messages/import-preview-dialog";

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
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    expect(previewDialog.textContent).toContain("Dropped Factory");
    expect(previewDialog.textContent).toContain("factory-import.png");
    expect(previewDialog.textContent).toContain(messages.hint);
    expect(
      within(previewDialog)
        .getByRole("img", { name: messages.previewImageAlt("Dropped Factory") })
        .getAttribute("src"),
    ).toBe("blob:factory-preview");

    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.cancelAction }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("has no accessibility violations in the ready preview state", async () => {
    const { baseElement } = render(
      <DashboardImportPreviewDialog
        activationState={{ status: "idle" }}
        importPreviewState={createReadyImportPreviewState()}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );
    await screen.findByRole("dialog", {
      name: getImportPreviewDialogMessages("en").title,
    });

    const results = await axe(baseElement);

    expect(results.violations).toEqual([]);
  });

  it("has no accessibility violations when activation fails", async () => {
    const { baseElement } = render(
      <DashboardImportPreviewDialog
        activationState={{
          error: new NamedFactoryAPIError("Network unreachable", { code: "NETWORK_ERROR" }),
          status: "error",
        }}
        importPreviewState={createReadyImportPreviewState()}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );
    const messages = getImportPreviewDialogMessages("en");
    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    expect(within(previewDialog).getByRole("alert")).toBeTruthy();

    const results = await axe(baseElement);

    expect(results.violations).toEqual([]);
  });

  it("blocks close interactions while activation is submitting", async () => {
    const { onCancel } = renderDialog({
      activationState: { status: "submitting" },
    });
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });
    const closeButton = within(previewDialog).getByRole("button", {
      name: messages.closeLabel,
    });
    const cancelButton = within(previewDialog).getByRole("button", {
      name: messages.cancelAction,
    });
    const activateButton = within(previewDialog).getByRole("button", {
      name: messages.activatingAction,
    });

    fireEvent.click(closeButton);
    fireEvent.click(cancelButton);

    expect(closeButton.getAttribute("disabled")).not.toBeNull();
    expect(cancelButton.getAttribute("disabled")).not.toBeNull();
    expect(activateButton.getAttribute("aria-busy")).toBe("true");
    expect(activateButton.getAttribute("disabled")).not.toBeNull();
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
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });
    expect(previewDialog.textContent).toContain(messages.activationErrorTitle);

    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.cancelAction }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: messages.title })).toBeNull();
    });
  });

  it("shows activation failures and delegates confirmation with the ready preview payload", async () => {
    const { onConfirm } = renderDialog({
      activationState: {
        error: new NamedFactoryAPIError("Network unreachable", { code: "NETWORK_ERROR" }),
        status: "error",
      },
    });
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    expect(previewDialog.textContent).toContain(messages.activationErrorTitle);
    expect(previewDialog.textContent).toContain(messages.errorByCode.NETWORK_ERROR);

    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.activateAction }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({ name: "Dropped Factory" }),
        }),
      );
    });
  });

  it.each([
    [
      "FACTORY_ALREADY_EXISTS",
      "Named factory already exists.",
      "A factory with this name already exists. Rename or remove the existing factory before importing this PNG.",
    ],
    [
      "FACTORY_NOT_IDLE",
      "Current factory runtime must be idle before activation.",
      "The current factory runtime is still active. Wait until it becomes idle before switching factories.",
    ],
    [
      "INVALID_FACTORY",
      "Dropped factory payload was rejected.",
      "The dropped factory payload was rejected by the activation API.",
    ],
    [
      "INVALID_FACTORY_NAME",
      "Embedded factory name is invalid.",
      "The embedded factory name is not valid for activation.",
    ],
    [
      "INTERNAL_ERROR",
      "Activation failed in an unexpected way.",
      "Activation failed in an unexpected way.",
    ],
  ] as const)(
    "renders the mapped activation copy for %s errors",
    async (code, message, expectedCopy) => {
      renderDialog({
        activationState: {
          error: new NamedFactoryAPIError(message, { code }),
          status: "error",
        },
      });
      const messages = getImportPreviewDialogMessages("en");

      const previewDialog = await screen.findByRole("dialog", { name: messages.title });
      const alert = within(previewDialog).getByRole("alert");

      expect(alert.textContent).toContain(messages.activationErrorTitle);
      expect(alert.textContent).toContain(expectedCopy);
    },
  );

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
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });
    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.activateAction }));

    await waitFor(() => {
      expect(activateImport).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({ name: "Dropped Factory" }),
        }),
      );
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: messages.title })).toBeNull();
    });
  });

  it("renders the localized dialog copy and controls for a non-default locale", async () => {
    const { onCancel } = renderDialog({ locale: "ja" });
    const messages = getImportPreviewDialogMessages("ja");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });
    const scope = within(previewDialog);

    expect(scope.getByRole("img", { name: messages.previewImageAlt("Dropped Factory") })).toBeTruthy();
    expect(scope.getByText("factory-import.png")).toBeTruthy();
    expect(scope.getByText(messages.hint)).toBeTruthy();
    expect(scope.getByRole("button", { name: messages.cancelAction })).toBeTruthy();
    expect(scope.getByRole("button", { name: messages.activateAction })).toBeTruthy();
    expect(scope.getByRole("button", { name: messages.closeLabel })).toBeTruthy();

    fireEvent.click(scope.getByRole("button", { name: messages.cancelAction }));

    expect(onCancel).toHaveBeenCalledTimes(1);
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
    const messages = getImportPreviewDialogMessages("en");

    expect(screen.queryByRole("dialog", { name: messages.title })).toBeNull();
  });
});
