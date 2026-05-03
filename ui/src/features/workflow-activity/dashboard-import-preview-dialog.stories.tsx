import { useState } from "react";

import { expect, userEvent, within } from "storybook/test";

import type { CurrentActivityImportController } from "./current-activity-import-controller";
import { DashboardImportPreviewDialog } from "./dashboard-import-preview-dialog";

const noop = () => {};

function createImportController(
  overrides: Partial<CurrentActivityImportController> = {},
): CurrentActivityImportController {
  return {
    activateImport: async () => undefined,
    activationState: { status: "idle" },
    clearActivationError: noop,
    clearError: noop,
    closeImportPreview: noop,
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
        revokePreviewImageSrc: noop,
        schemaVersion: "portos.agent-factory.png.v1",
      },
    },
    onDragEnter: noop,
    onDragLeave: noop,
    onDragOver: noop,
    onDrop: noop,
    ...overrides,
  };
}

function ImportPreviewStory() {
  const [activationStatus, setActivationStatus] = useState("No factory activated yet.");
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
        revokePreviewImageSrc: noop,
        schemaVersion: "portos.agent-factory.png.v1",
      },
    });

  return (
    <>
      <DashboardImportPreviewDialog
        importController={createImportController({
          activateImport: async () => {
            setActivationStatus("Activated factory: Dropped Factory");
            setImportPreviewState({ status: "idle" });
          },
          closeImportPreview: () => {
            setActivationStatus("Import preview dismissed.");
            setImportPreviewState({ status: "idle" });
          },
          importPreviewState,
        })}
      />
      <p>{activationStatus}</p>
    </>
  );
}

export default {
  title: "Agent Factory/Dashboard/Import Preview Dialog",
  component: DashboardImportPreviewDialog,
  tags: ["test"],
};

export const Ready = {
  render: () => <ImportPreviewStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    const dialog = await page.findByRole("dialog", { name: "Review factory import" });
    const scope = within(dialog);
    const cancelButton = scope.getByRole("button", { name: "Cancel import" });
    const activateButton = scope.getByRole("button", { name: "Activate factory" });
    const closeButton = scope.getByRole("button", { name: "Close import preview" });

    await expect(scope.getByRole("img", { name: "Dropped Factory preview" })).toBeVisible();
    await expect(scope.getByText("factory-import.png")).toBeVisible();
    await expect(
      scope.getByText("Activating the import switches the current dashboard factory to the embedded authored definition from this PNG."),
    ).toBeVisible();

    cancelButton.focus();
    await expect(cancelButton).toHaveFocus();
    await userEvent.tab();
    await expect(activateButton).toHaveFocus();
    await userEvent.tab();
    await expect(closeButton).toHaveFocus();
  },
};
