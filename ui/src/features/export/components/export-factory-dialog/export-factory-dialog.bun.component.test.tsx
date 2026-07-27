import { expect, it, mock } from "bun:test";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { ComponentProps } from "react";

import type { WriteFactoryExportPngResult } from "../../lib/factory-png-export";
import { getExportDialogMessages } from "../../messages/export-dialog";
import {
  ExportFactoryDialog,
  type ExportFactoryDialogEffects,
} from "../export-factory-dialog";

const factory = {
  name: "Factory Aurora",
  workspaces: {},
} as const;

function successfulExportResult(): WriteFactoryExportPngResult {
  return {
    blob: new Blob(["png"], { type: "image/png" }),
    metadata: {
      schemaVersion: "portos.agent-factory.png.v1",
      ...factory,
    },
    ok: true,
  };
}

function createEffects(
  writeFactoryExportPng: ExportFactoryDialogEffects["writeFactoryExportPng"] = async () =>
    successfulExportResult(),
) {
  const downloadBlobAsFile = mock(
    (
      _options: Parameters<ExportFactoryDialogEffects["downloadBlobAsFile"]>[0],
    ) => {},
  );

  return {
    downloadBlobAsFile,
    effects: {
      downloadBlobAsFile,
      writeFactoryExportPng: mock(writeFactoryExportPng),
    } satisfies ExportFactoryDialogEffects,
  };
}

function renderDialog(
  overrides: Partial<ComponentProps<typeof ExportFactoryDialog>> = {},
  effects = createEffects(),
) {
  return {
    ...effects,
    ...render(
      <ExportFactoryDialog
        effects={effects.effects}
        factory={factory}
        initialFactoryName="Factory Aurora"
        isOpen
        onClose={() => {}}
        {...overrides}
      />,
    ),
  };
}

it("ExportFactoryDialog validates required fields and rejects non-image selections", async () => {
  const messages = getExportDialogMessages("en");
  const { effects } = renderDialog({ initialFactoryName: "" });
  const exportAction = screen.getByRole("button", {
    name: messages.exportAction,
  });

  fireEvent.click(exportAction);

  const nameValidation = await screen.findByText(
    messages.nameRequiredValidation,
  );
  const imageValidation = screen.getByText(messages.imageRequiredValidation);
  const nameInput = screen.getByLabelText(messages.nameLabel);
  const imageInput = screen.getByLabelText(messages.imageLabel);

  expect(nameInput.getAttribute("aria-describedby")).toBe(nameValidation.id);
  expect(imageInput.getAttribute("aria-describedby")).toBe(imageValidation.id);

  fireEvent.change(imageInput, {
    target: {
      files: [new File(["notes"], "notes.txt", { type: "text/plain" })],
    },
  });

  expect(await screen.findByText(messages.imageTypeValidation)).toBeTruthy();
  expect(effects.writeFactoryExportPng).not.toHaveBeenCalled();
});

it("ExportFactoryDialog blocks export while preparation is pending or unavailable", () => {
  const messages = getExportDialogMessages("en");
  const { rerender } = renderDialog({ isPreparing: true });

  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: messages.exportAction,
    }).disabled,
  ).toBe(true);
  expect(screen.getByRole("status").textContent).toContain(
    messages.loadingStatus,
  );

  rerender(
    <ExportFactoryDialog
      factory={null}
      initialFactoryName="you-agent-factory"
      isOpen
      onClose={() => {}}
      preparationFailure={{
        code: "FACTORY_DEFINITION_UNAVAILABLE",
        message: "Factory unavailable",
        ok: false,
      }}
    />,
  );

  expect(screen.getByRole("status").textContent).toContain(
    "Factory unavailable",
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: messages.exportAction,
    }).disabled,
  ).toBe(true);
});

it("ExportFactoryDialog exports the selected image with a trimmed name and visible success state", async () => {
  let resolveExport: ((result: WriteFactoryExportPngResult) => void) | null =
    null;
  const effects = createEffects(
    () =>
      new Promise((resolve) => {
        resolveExport = resolve;
      }),
  );
  const messages = getExportDialogMessages("en");
  const exportImage = new File(["binary"], "cover.png", {
    type: "image/png",
  });
  renderDialog({ initialFactoryName: "  Factory Aurora  " }, effects);

  const imageInput = screen.getByLabelText<HTMLInputElement>(
    messages.imageLabel,
  );
  fireEvent.change(imageInput, { target: { files: [exportImage] } });
  expect(
    screen.getByText(messages.selectedImageLabel("cover.png")),
  ).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: messages.exportAction }));

  const exportingAction = await screen.findByRole<HTMLButtonElement>("button", {
    name: messages.exportingAction,
  });
  expect(exportingAction.getAttribute("aria-busy")).toBe("true");
  expect(imageInput.disabled).toBe(true);
  expect(effects.effects.writeFactoryExportPng).toHaveBeenCalledWith({
    factory: { ...factory, name: "Factory Aurora" },
    image: exportImage,
  });

  if (!resolveExport) {
    throw new Error("expected the export request to be pending");
  }
  resolveExport(successfulExportResult());

  await waitFor(() => {
    expect(effects.downloadBlobAsFile).toHaveBeenCalledWith({
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
});

it("ExportFactoryDialog keeps the dialog open and reports export failures", async () => {
  const effects = createEffects(async () => ({
    error: {
      code: "PNG_METADATA_WRITE_FAILED",
      message: "PNG encoding failed",
    },
    ok: false,
  }));
  const onClose = mock(() => {});
  const messages = getExportDialogMessages("en");
  renderDialog({ onClose }, effects);

  fireEvent.change(screen.getByLabelText(messages.imageLabel), {
    target: {
      files: [new File(["binary"], "cover.png", { type: "image/png" })],
    },
  });
  fireEvent.click(screen.getByRole("button", { name: messages.exportAction }));

  expect((await screen.findByRole("alert")).textContent).toContain(
    "PNG encoding failed",
  );
  expect(screen.getByRole("dialog", { name: messages.title })).toBeTruthy();
  expect(effects.downloadBlobAsFile).not.toHaveBeenCalled();
  expect(onClose).not.toHaveBeenCalled();
});

it("ExportFactoryDialog preserves a typed name when the prepared factory name refreshes", async () => {
  const messages = getExportDialogMessages("en");
  const { effects, rerender } = renderDialog({
    initialFactoryName: "Browser Export Factory",
  });

  fireEvent.change(screen.getByLabelText(messages.nameLabel), {
    target: { value: "Roundtrip Browser Export" },
  });
  rerender(
    <ExportFactoryDialog
      effects={effects}
      factory={factory}
      initialFactoryName="Renamed Browser Export Factory"
      isOpen
      onClose={() => {}}
    />,
  );

  await waitFor(() => {
    expect(
      screen.getByLabelText<HTMLInputElement>(messages.nameLabel).value,
    ).toBe("Roundtrip Browser Export");
  });
});

it("ExportFactoryDialog ignores an export that completes after the dialog closes", async () => {
  let resolveExport: ((result: WriteFactoryExportPngResult) => void) | null =
    null;
  const effects = createEffects(
    () =>
      new Promise((resolve) => {
        resolveExport = resolve;
      }),
  );
  const onClose = mock(() => {});
  const messages = getExportDialogMessages("en");
  renderDialog({ onClose }, effects);

  fireEvent.change(screen.getByLabelText(messages.imageLabel), {
    target: {
      files: [new File(["binary"], "cover.png", { type: "image/png" })],
    },
  });
  fireEvent.click(screen.getByRole("button", { name: messages.exportAction }));
  await screen.findByRole("button", { name: messages.exportingAction });
  fireEvent.click(screen.getByRole("button", { name: messages.cancelAction }));

  expect(onClose).toHaveBeenCalledTimes(1);
  if (!resolveExport) {
    throw new Error("expected the export request to be pending");
  }
  resolveExport(successfulExportResult());

  await waitFor(() => {
    expect(effects.effects.writeFactoryExportPng).toHaveBeenCalledTimes(1);
  });
  expect(effects.downloadBlobAsFile).not.toHaveBeenCalled();
  expect(
    screen.queryByText(messages.successMessage("factory-aurora.png")),
  ).toBeNull();
});

it("ExportFactoryDialog renders localized controls and validation copy", async () => {
  const messages = getExportDialogMessages("zh-CN");
  renderDialog({ initialFactoryName: "", locale: "zh-CN" });

  const dialog = screen.getByRole("dialog", { name: messages.title });
  expect(screen.getByText(messages.description)).toBeTruthy();
  expect(screen.getByLabelText(messages.nameLabel)).toBeTruthy();
  expect(screen.getByLabelText(messages.imageLabel)).toBeTruthy();
  expect(
    within(dialog).getByRole("button", { name: messages.closeLabel }),
  ).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: messages.exportAction }));
  expect(await screen.findByText(messages.nameRequiredValidation)).toBeTruthy();
  expect(screen.getByText(messages.imageRequiredValidation)).toBeTruthy();
});
