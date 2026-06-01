import { waitFor } from "@testing-library/react";
import { expect } from "vitest";
import { toast } from "sonner";

import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../../notifications/public";

export const WORKSTATION_SAVE_SUCCESS_DESCRIPTION =
  "Running factory saved. The editable workstation values were refreshed to the saved definition.";

export const WORKSTATION_STALE_VERSION_DETAIL =
  "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.";

export async function expectWorkstationSaveSuccessToast() {
  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      "Workstation saved",
      expect.objectContaining({
        description: WORKSTATION_SAVE_SUCCESS_DESCRIPTION,
      }),
    );
  });
}

export async function expectWorkstationSaveFailedToast(errorDetail: string) {
  await waitFor(() => {
    expect(toast.error).toHaveBeenCalledWith(
      "Workstation save failed",
      expect.objectContaining({
        description: expect.stringContaining(errorDetail),
        duration: PERSISTENT_TOAST_DURATION_MS,
      }),
    );
  });
}

export async function expectWorkstationStaleSaveWarningToast(
  warningTitle: string,
) {
  await waitFor(() => {
    expect(toast.warning).toHaveBeenCalledWith(
      warningTitle,
      expect.objectContaining({
        description: WORKSTATION_STALE_VERSION_DETAIL,
        duration: GLOBAL_TOAST_DURATION_MS,
      }),
    );
  });
}

export async function expectWorkerSaveSuccessToast(workerName: string) {
  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      "Worker saved",
      expect.objectContaining({
        description: expect.stringContaining(
          `${workerName} was updated in the running factory definition`,
        ),
      }),
    );
  });
}

export async function expectWorkerSaveFailedToast(errorDetail: string) {
  await waitFor(() => {
    expect(toast.error).toHaveBeenCalledWith(
      "Worker save failed",
      expect.objectContaining({
        description: expect.stringContaining(errorDetail),
        duration: PERSISTENT_TOAST_DURATION_MS,
      }),
    );
  });
}

export async function expectResourceSaveSuccessToast(resourceName: string) {
  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      "Resource saved",
      expect.objectContaining({
        description: expect.stringContaining(resourceName),
      }),
    );
  });
}

export async function expectWorkStateSaveSuccessToast(stateName: string) {
  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      "Work state saved",
      expect.objectContaining({
        description: expect.stringContaining(
          `${stateName} was updated in the running factory definition`,
        ),
      }),
    );
  });
}
