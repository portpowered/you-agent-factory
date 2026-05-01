import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { ExportFactoryDialog } from "./export-factory-dialog";
import type { WriteFactoryExportPngResult } from "./factory-png-export";
import { downloadBlobAsFile } from "./browser-download";
import { writeFactoryExportPng } from "./factory-png-export";

vi.mock("./browser-download", () => ({
  downloadBlobAsFile: vi.fn(),
}));

vi.mock("./factory-png-export", () => ({
  writeFactoryExportPng: vi.fn(),
}));

const namedFactory = {
  factory: {
    workspaces: {},
  },
  name: "Factory Aurora",
} as const;

describe("ExportFactoryDialog", () => {
  beforeEach(() => {
    vi.mocked(downloadBlobAsFile).mockReset();
    vi.mocked(writeFactoryExportPng).mockReset();
  });

  it("shows validation errors when the export name or cover image is missing", async () => {
    render(
      <ExportFactoryDialog
        initialFactoryName=""
        isOpen
        namedFactory={namedFactory}
        onClose={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

    const nameValidation = await screen.findByText("Enter a factory name before exporting.");
    const imageValidation = screen.getByText("Choose a cover image before exporting.");
    const nameInput = screen.getByLabelText("Factory name");
    const imageInput = screen.getByLabelText("Cover image");

    expect(nameValidation.id).not.toBe("");
    expect(imageValidation.id).not.toBe("");
    expect(nameInput.getAttribute("aria-invalid")).toBe("true");
    expect(nameInput.getAttribute("aria-describedby")).toBe(nameValidation.id);
    expect(imageInput.getAttribute("aria-invalid")).toBe("true");
    expect(imageInput.getAttribute("aria-describedby")).toBe(imageValidation.id);
    expect(writeFactoryExportPng).not.toHaveBeenCalled();
  });

  it("disables actions while the export is being prepared", () => {
    render(
      <ExportFactoryDialog
        initialFactoryName="Factory Aurora"
        isOpen
        isPreparing
        namedFactory={namedFactory}
        onClose={() => {}}
      />,
    );

    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Export PNG" }).disabled).toBe(
      true,
    );
    expect(screen.getByText("Loading the current authored factory definition.")).toBeTruthy();
  });

  it("exports the selected image with the trimmed factory name and closes the dialog", async () => {
    let resolveExport: ((value: WriteFactoryExportPngResult) => void) | null = null;
    vi.mocked(writeFactoryExportPng).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExport = resolve;
        }),
    );
    const onClose = vi.fn();
    render(
      <ExportFactoryDialog
        initialFactoryName="  Factory Aurora  "
        isOpen
        namedFactory={namedFactory}
        onClose={onClose}
      />,
    );

    const fileInput = screen.getByLabelText("Cover image");
    const exportImage = new File(["binary"], "cover.png", { type: "image/png" });
    fireEvent.change(fileInput, { target: { files: [exportImage] } });
    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Exporting..." })).toBeTruthy();
    });
    expect(screen.getByRole("button", { name: "Exporting..." }).getAttribute("aria-busy")).toBe(
      "true",
    );
    expect(writeFactoryExportPng).toHaveBeenCalledWith({
      image: exportImage,
      namedFactory: {
        ...namedFactory,
        name: "Factory Aurora",
      },
    });

    if (!resolveExport) {
      throw new Error("expected export request promise to be pending");
    }

    resolveExport({
      blob: new Blob(["png"], { type: "image/png" }),
      envelope: {
        ...namedFactory,
        name: "Factory Aurora",
        schemaVersion: "portos.agent-factory.png.v1",
      },
      ok: true,
    });

    await waitFor(() => {
      expect(downloadBlobAsFile).toHaveBeenCalledWith({
        blob: expect.any(Blob),
        filename: "factory-aurora.png",
      });
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
