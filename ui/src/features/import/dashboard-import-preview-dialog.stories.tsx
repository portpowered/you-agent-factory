import { useState } from "react";

import { expect, userEvent, within } from "storybook/test";

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
      revokePreviewImageSrc: () => {},
      schemaVersion: "portos.agent-factory.png.v1",
    },
  };
}

function ImportPreviewStory() {
  const [activationStatus, setActivationStatus] = useState("No factory activated yet.");
  const [importPreviewState, setImportPreviewState] =
    useState<DashboardImportPreviewDialogProps["importPreviewState"]>(
      createReadyImportPreviewState(),
    );

  return (
    <>
      <DashboardImportPreviewDialog
        activationState={{ status: "idle" }}
        importPreviewState={importPreviewState}
        onCancel={() => {
          setActivationStatus("Import preview dismissed.");
          setImportPreviewState({ status: "idle" });
        }}
        onConfirm={async () => {
          setActivationStatus("Activated factory: Dropped Factory");
          setImportPreviewState({ status: "idle" });
        }}
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
