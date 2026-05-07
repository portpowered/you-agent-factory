import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ComponentProps } from "react";
import { downloadBlobAsFile } from "./browser-download";
import { ExportFactoryDialog } from "./export-factory-dialog";
import type { WriteFactoryExportPngResult } from "./factory-png-export";
import { writeFactoryExportPng } from "./factory-png-export";
import { getExportDialogMessages } from "./messages/export-dialog";

vi.mock("./browser-download", () => ({
  downloadBlobAsFile: vi.fn(),
}));

vi.mock("./factory-png-export", () => ({
  writeFactoryExportPng: vi.fn(),
}));

const factory = {
  name: "Factory Aurora",
  workspaces: {},
} as const;

function renderDialog(
  overrides: Partial<ComponentProps<typeof ExportFactoryDialog>> = {},
) {
  return render(
    <ExportFactoryDialog
      factory={factory}
      initialFactoryName="Factory Aurora"
      isOpen
      onClose={() => {}}
      {...overrides}
    />,
  );
}

describe("ExportFactoryDialog", () => {
  beforeEach(() => {
    vi.mocked(downloadBlobAsFile).mockReset();
    vi.mocked(writeFactoryExportPng).mockReset();
  });

  it("shows validation errors when the export name or cover image is missing", async () => {
    const messages = getExportDialogMessages("en");
    renderDialog({ initialFactoryName: "" });

    fireEvent.click(
      screen.getByRole("button", { name: messages.exportAction }),
    );

    const nameValidation = await screen.findByText(
      messages.nameRequiredValidation,
    );
    const imageValidation = screen.getByText(messages.imageRequiredValidation);
    const nameInput = screen.getByLabelText(messages.nameLabel);
    const imageInput = screen.getByLabelText(messages.imageLabel);

    expect(nameValidation.id).not.toBe("");
    expect(imageValidation.id).not.toBe("");
    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(nameInput.getAttribute("aria-describedby")).toBe(nameValidation.id);
    expect(imageInput.getAttribute("aria-invalid")).toBe("true");
    expect(imageInput.getAttribute("aria-describedby")).toBe(
      imageValidation.id,
    );
    expect(writeFactoryExportPng).not.toHaveBeenCalled();
  });

  it("validates cleared and non-image file selections before export", async () => {
    const messages = getExportDialogMessages("en");
    renderDialog();

    const imageInput = screen.getByLabelText(messages.imageLabel);
    fireEvent.change(imageInput, { target: { files: [] } });

    expect(
      await screen.findByText(messages.imageRequiredValidation),
    ).toBeTruthy();
    expect(imageInput.getAttribute("aria-invalid")).toBe("true");

    fireEvent.change(imageInput, {
      target: {
        files: [new File(["notes"], "notes.txt", { type: "text/plain" })],
      },
    });

    expect(await screen.findByText(messages.imageTypeValidation)).toBeTruthy();
    expect(writeFactoryExportPng).not.toHaveBeenCalled();
  });

  it("disables actions while the export is being prepared", () => {
    const messages = getExportDialogMessages("en");
    renderDialog({ isPreparing: true });

    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: messages.exportAction,
      }).disabled,
    ).toBe(true);
    expect(screen.getByText(messages.loadingStatus)).toBeTruthy();
  });

  it("renders Infinite You export copy for metadata and filename guidance", () => {
    const messages = getExportDialogMessages("en");
    renderDialog();

    expect(screen.getByText(messages.hint)).toBeTruthy();
    expect(screen.getByText(messages.nameDescription)).toBeTruthy();
  });

  it("exports the selected image with the trimmed factory name and shows a visible success state", async () => {
    let resolveExport: ((value: WriteFactoryExportPngResult) => void) | null =
      null;
    vi.mocked(writeFactoryExportPng).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExport = resolve;
        }),
    );
    const onClose = vi.fn();
    render(
      <ExportFactoryDialog
        factory={factory}
        initialFactoryName="  Factory Aurora  "
        isOpen
        onClose={onClose}
      />,
    );
    const messages = getExportDialogMessages("en");

    const fileInput = screen.getByLabelText(messages.imageLabel);
    const exportImage = new File(["binary"], "cover.png", {
      type: "image/png",
    });
    fireEvent.change(fileInput, { target: { files: [exportImage] } });
    fireEvent.click(
      screen.getByRole("button", { name: messages.exportAction }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.exportingAction }),
      ).toBeTruthy();
    });
    expect(
      screen
        .getByRole("button", { name: messages.exportingAction })
        .getAttribute("aria-busy"),
    ).toBe("true");
    expect(writeFactoryExportPng).toHaveBeenCalledWith({
      factory: {
        ...factory,
        name: "Factory Aurora",
      },
      image: exportImage,
    });

    if (!resolveExport) {
      throw new Error("expected export request promise to be pending");
    }

    resolveExport({
      blob: new Blob(["png"], { type: "image/png" }),
      envelope: {
        schemaVersion: "portos.agent-factory.png.v1",
        ...factory,
        name: "Factory Aurora",
      },
      ok: true,
    });

    await waitFor(() => {
      expect(downloadBlobAsFile).toHaveBeenCalledWith({
        blob: expect.any(Blob),
        filename: "factory-aurora.png",
      });
    });
    expect(screen.getByRole("status").textContent).toContain(
      messages.successMessage("factory-aurora.png"),
    );
    expect(
      screen.getByRole("button", { name: messages.closeAction }),
    ).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a visible export failure without closing the shared dialog", async () => {
    vi.mocked(writeFactoryExportPng).mockResolvedValue({
      error: new Error("PNG encoding failed"),
      ok: false,
    });
    const onClose = vi.fn();
    render(
      <ExportFactoryDialog
        factory={factory}
        initialFactoryName="Factory Aurora"
        isOpen
        onClose={onClose}
      />,
    );
    const messages = getExportDialogMessages("en");

    fireEvent.change(screen.getByLabelText(messages.imageLabel), {
      target: {
        files: [new File(["binary"], "cover.png", { type: "image/png" })],
      },
    });
    fireEvent.click(
      screen.getByRole("button", { name: messages.exportAction }),
    );

    const errorPanel = await screen.findByRole("alert");
    expect(errorPanel.textContent).toContain("PNG encoding failed");
    expect(screen.getByRole("dialog", { name: messages.title })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.cancelAction }),
    ).toBeTruthy();
    expect(downloadBlobAsFile).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("preserves a typed export name when the initial factory name refreshes mid-dialog", async () => {
    const { rerender } = render(
      <ExportFactoryDialog
        factory={factory}
        initialFactoryName="Browser Export Factory"
        isOpen
        onClose={() => {}}
      />,
    );

    const nameInput = screen.getByLabelText(
      getExportDialogMessages("en").nameLabel,
    );
    fireEvent.change(nameInput, {
      target: { value: "Roundtrip Browser Export" },
    });
    expect((nameInput as HTMLInputElement).value).toBe(
      "Roundtrip Browser Export",
    );

    rerender(
      <ExportFactoryDialog
        factory={factory}
        initialFactoryName="Renamed Browser Export Factory"
        isOpen
        onClose={() => {}}
      />,
    );

    await waitFor(() => {
      expect(
        (
          screen.getByLabelText(
            getExportDialogMessages("en").nameLabel,
          ) as HTMLInputElement
        ).value,
      ).toBe("Roundtrip Browser Export");
    });
  });

  it("surfaces the preparation failure and blocks export when the factory is unavailable", async () => {
    const messages = getExportDialogMessages("en");
    renderDialog({
      factory: null,
      initialFactoryName: "infinite-you",
      preparationFailure: {
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message:
          "The current factory definition could not be loaded from the current-factory API.",
        ok: false,
      },
    });

    const errorPanel = await screen.findByRole("status");
    expect(errorPanel.textContent).toContain(
      "The current factory definition could not be loaded from the current-factory API.",
    );
    expect(
      (
        screen.getByRole("button", {
          name: messages.exportAction,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(writeFactoryExportPng).not.toHaveBeenCalled();
    expect(downloadBlobAsFile).not.toHaveBeenCalled();
  });

  it("ignores a completed export after the dialog was closed mid-flight", async () => {
    let resolveExport: ((value: WriteFactoryExportPngResult) => void) | null =
      null;
    vi.mocked(writeFactoryExportPng).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExport = resolve;
        }),
    );
    const onClose = vi.fn();
    render(
      <ExportFactoryDialog
        factory={factory}
        initialFactoryName="Factory Aurora"
        isOpen
        onClose={onClose}
      />,
    );
    const messages = getExportDialogMessages("en");

    fireEvent.change(screen.getByLabelText(messages.imageLabel), {
      target: {
        files: [new File(["binary"], "cover.png", { type: "image/png" })],
      },
    });
    fireEvent.click(
      screen.getByRole("button", { name: messages.exportAction }),
    );

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: messages.exportingAction }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: messages.cancelAction }),
    );

    expect(onClose).toHaveBeenCalledTimes(1);

    if (!resolveExport) {
      throw new Error("expected export request promise to be pending");
    }

    resolveExport({
      blob: new Blob(["png"], { type: "image/png" }),
      envelope: {
        schemaVersion: "portos.agent-factory.png.v1",
        ...factory,
      },
      ok: true,
    });

    await waitFor(() => {
      expect(writeFactoryExportPng).toHaveBeenCalledTimes(1);
    });
    expect(downloadBlobAsFile).not.toHaveBeenCalled();
    expect(
      screen.queryByText(messages.successMessage("factory-aurora.png")),
    ).toBeNull();
  });

  it("renders the localized export dialog surface for a non-default locale", async () => {
    const messages = getExportDialogMessages("ja");
    renderDialog({ locale: "ja" });

    const dialog = await screen.findByRole("dialog", { name: messages.title });

    expect(screen.getByText(messages.description)).toBeTruthy();
    expect(screen.getByText(messages.hint)).toBeTruthy();
    expect(screen.getByLabelText(messages.nameLabel)).toBeTruthy();
    expect(screen.getByLabelText(messages.imageLabel)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.cancelAction }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: messages.exportAction }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: messages.closeLabel }),
    ).toBeTruthy();
  });
});
