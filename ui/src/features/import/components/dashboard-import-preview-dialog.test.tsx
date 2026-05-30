import { useState } from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { axe } from "jest-axe";

import {
  type FactoryImportSaveChoice,
  NamedFactoryAPIError,
} from "../../../api/named-factory";
import {
  DashboardImportPreviewDialog,
  FactoryImportPreviewDialog,
  type DashboardImportPreviewDialogProps,
} from "../public";
import { getImportPreviewDialogMessages } from "../messages/import-preview-dialog";

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

const defaultCurrentSessionFactoryName = "alpha";
const defaultCreateTargetFactoryName = "Dropped Factory-2";

function ImportPreviewDialogHarness({
  overrides = {},
}: {
  overrides?: Partial<DashboardImportPreviewDialogProps>;
}) {
  const [importSaveChoice, setImportSaveChoice] = useState<FactoryImportSaveChoice>(
    overrides.importSaveChoice ?? "REPLACE_CURRENT",
  );

  return (
    <DashboardImportPreviewDialog
      activationState={overrides.activationState ?? { status: "idle" }}
      createTargetFactoryName={
        overrides.createTargetFactoryName ?? defaultCreateTargetFactoryName
      }
      currentFactoryName={
        overrides.currentFactoryName ?? defaultCurrentSessionFactoryName
      }
      importPreviewState={
        overrides.importPreviewState ?? createReadyImportPreviewState()
      }
      importSaveChoice={importSaveChoice}
      locale={overrides.locale}
      onCancel={overrides.onCancel ?? vi.fn()}
      onConfirm={overrides.onConfirm ?? vi.fn().mockResolvedValue(undefined)}
      onImportSaveChoiceChange={setImportSaveChoice}
      sessionID={overrides.sessionID}
    />
  );
}

function renderDialog(
  overrides: Partial<DashboardImportPreviewDialogProps> = {},
) {
  const onCancel = vi.fn();
  const onConfirm = vi.fn().mockResolvedValue(undefined);

  render(
    <ImportPreviewDialogHarness
      overrides={{
        ...overrides,
        onCancel: overrides.onCancel ?? onCancel,
        onConfirm: overrides.onConfirm ?? onConfirm,
      }}
    />,
  );

  return { onCancel: overrides.onCancel ?? onCancel, onConfirm: overrides.onConfirm ?? onConfirm };
}

describe("DashboardImportPreviewDialog", () => {
  it("renders the factory preview dialog through the import public boundary", async () => {
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    const messages = getImportPreviewDialogMessages("en");

    render(
      <FactoryImportPreviewDialog
        activationState={{ status: "idle" }}
        currentFactoryName={defaultCurrentSessionFactoryName}
        importSaveChoice="REPLACE_CURRENT"
        onCancel={onCancel}
        onConfirm={onConfirm}
        onImportSaveChoiceChange={vi.fn()}
        previewState={createReadyImportPreviewState()}
      />,
    );

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.cancelAction }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

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
    expect(
      within(
        within(previewDialog)
          .getByText(messages.embeddedFactoryLabel)
          .closest("div") as HTMLElement,
      )
        .getByText("Dropped Factory")
        .className,
    ).toContain("text-af-text");

    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.cancelAction }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("has no accessibility violations in the ready preview state", async () => {
    const { baseElement } = render(<ImportPreviewDialogHarness />);
    await screen.findByRole("dialog", {
      name: getImportPreviewDialogMessages("en").title,
    });

    const results = await axe(baseElement);

    expect(results.violations).toEqual([]);
  });

  it("has no accessibility violations when activation fails", async () => {
    const { baseElement } = render(
      <ImportPreviewDialogHarness
        overrides={{
          activationState: {
            error: new NamedFactoryAPIError("Network unreachable", {
              code: "NETWORK_ERROR",
            }),
            status: "error",
          },
        }}
      />,
    );
    const messages = getImportPreviewDialogMessages("en");
    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    const alert = within(previewDialog).getByRole("alert");
    expect(alert).toBeTruthy();
    expect(alert.className).toContain("border-af-danger-border");
    expect(alert.className).toContain("bg-af-danger-surface");

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
        <ImportPreviewDialogHarness
          overrides={{
            activationState,
            importPreviewState,
            onCancel: () => {
              setActivationState({ status: "idle" });
              setImportPreviewState({ status: "idle" });
            },
          }}
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

  it("defaults to replace current factory and exposes a keyboard-accessible save choice group", async () => {
    renderDialog();
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });
    const replaceOption = within(previewDialog).getByRole("button", {
      name: new RegExp(messages.replaceCurrentFactoryLabel),
    });
    const createOption = within(previewDialog).getByRole("button", {
      name: new RegExp(messages.createNewNamedFactoryLabel),
    });

    expect(replaceOption.getAttribute("aria-pressed")).toBe("true");
    expect(createOption.getAttribute("aria-pressed")).toBe("false");

    fireEvent.click(createOption);
    expect(createOption.getAttribute("aria-pressed")).toBe("true");
    expect(replaceOption.getAttribute("aria-pressed")).toBe("false");
    expect(previewDialog.textContent).toContain(
      `${messages.createNewNamedFactoryResolvedNameLabel}: Dropped Factory-2`,
    );

    fireEvent.click(replaceOption);
    expect(replaceOption.getAttribute("aria-pressed")).toBe("true");
    expect(previewDialog.textContent).toContain(
      messages.replaceCurrentFactoryDescription(defaultCurrentSessionFactoryName),
    );
  });

  it("shows a suffixed create path factory name when the embedded name collides", async () => {
    renderDialog({
      createTargetFactoryName: "Dropped Factory-2",
      currentFactoryName: "Dropped Factory",
    });
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    fireEvent.click(
      within(previewDialog).getByRole("button", {
        name: new RegExp(messages.createNewNamedFactoryLabel),
      }),
    );

    expect(previewDialog.textContent).toContain(
      `${messages.createNewNamedFactoryResolvedNameLabel}: Dropped Factory-2`,
    );
  });

  it("delegates confirmation with CREATE_NEW_NAMED when that save choice is selected", async () => {
    const { onConfirm } = renderDialog();
    const messages = getImportPreviewDialogMessages("en");

    const previewDialog = await screen.findByRole("dialog", { name: messages.title });

    fireEvent.click(
      within(previewDialog).getByRole("button", {
        name: new RegExp(messages.createNewNamedFactoryLabel),
      }),
    );
    fireEvent.click(within(previewDialog).getByRole("button", { name: messages.activateAction }));

    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith(
        expect.objectContaining({
          factory: expect.objectContaining({ name: "Dropped Factory" }),
        }),
        "CREATE_NEW_NAMED",
      );
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
        "REPLACE_CURRENT",
      );
    });
  });

  it.each([
    ["FACTORY_ALREADY_EXISTS", "Named factory already exists."],
    ["FACTORY_NOT_IDLE", "Current factory runtime must be idle before activation."],
    ["INVALID_FACTORY", "Dropped factory payload was rejected."],
    ["INVALID_FACTORY_NAME", "Embedded factory name is invalid."],
    ["STALE_FACTORY_VERSION", "The editable definition is stale."],
    ["INTERNAL_ERROR", "Activation failed in an unexpected way."],
  ] as const)(
    "renders the mapped activation copy for %s errors",
    async (code, message) => {
      const messages = getImportPreviewDialogMessages("en");
      const expectedCopy =
        code === "INTERNAL_ERROR" || code === "STALE_FACTORY_VERSION"
          ? message
          : messages.errorByCode[code as keyof typeof messages.errorByCode];

      renderDialog({
        activationState: {
          error: new NamedFactoryAPIError(message, { code }),
          status: "error",
        },
      });

      const previewDialog = await screen.findByRole("dialog", { name: messages.title });
      const alert = within(previewDialog).getByRole("alert");

      expect(
        within(alert).getByRole("heading", { level: 3 }).textContent,
      ).toContain(messages.activationErrorTitle);
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
        <ImportPreviewDialogHarness
          overrides={{
            importPreviewState,
            onCancel: () => {
              setImportPreviewState({ status: "idle" });
            },
            onConfirm: async (value, choice) => {
              await activateImport(value, choice);
              setImportPreviewState({ status: "idle" });
            },
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
        "REPLACE_CURRENT",
      );
    });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: messages.title })).toBeNull();
    });
  });

  it("renders the localized dialog copy and controls for zh-CN", async () => {
    const { onCancel } = renderDialog({ locale: "zh-CN" });
    const messages = getImportPreviewDialogMessages("zh-CN");

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

  it("renders mapped activation error copy for zh-CN", async () => {
    renderDialog({
      activationState: {
        error: new NamedFactoryAPIError("Network unreachable", {
          code: "NETWORK_ERROR",
        }),
        status: "error",
      },
      locale: "zh-CN",
    });
    const messages = getImportPreviewDialogMessages("zh-CN");

    const previewDialog = await screen.findByRole("dialog", {
      name: messages.title,
    });
    const alert = within(previewDialog).getByRole("alert");

    expect(alert.textContent).toContain(messages.activationErrorTitle);
    expect(alert.textContent).toContain(messages.errorByCode.NETWORK_ERROR);
  });

  it("does not render when no preview is ready", () => {
    render(
      <DashboardImportPreviewDialog
        activationState={{ status: "idle" }}
        currentFactoryName={defaultCurrentSessionFactoryName}
        importPreviewState={{ status: "idle" }}
        importSaveChoice="REPLACE_CURRENT"
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
        onImportSaveChoiceChange={vi.fn()}
      />,
    );
    const messages = getImportPreviewDialogMessages("en");

    expect(screen.queryByRole("dialog", { name: messages.title })).toBeNull();
  });
});
