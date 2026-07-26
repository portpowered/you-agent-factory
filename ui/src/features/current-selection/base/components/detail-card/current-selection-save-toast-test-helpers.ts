import { screen, waitFor, within } from "@testing-library/react";
import { toast } from "sonner";
import { settleCurrentSelectionEffects } from "../../../../../testing/current-selection-test-utils";
import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "../../../../notifications/lib/notification-toast-duration";
import { getWorkstationDetailMessages } from "../../../workstation-selection/messages/workstation-detail";
import { getCurrentSelectionGraphDraftConflictMessages } from "../../messages/operational/current-selection-graph-draft-conflict";

const graphDraftConflictMessages =
  getCurrentSelectionGraphDraftConflictMessages("en");
const workstationDetailMessages = getWorkstationDetailMessages("en");

export const WORKSTATION_STALE_VERSION_DETAIL =
  "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.";

const INLINE_SAVE_SUCCESS_PATTERN = /^Running factory saved\./;
const INLINE_SAVE_FAILURE_PREFIX = /^Saving failed\./;

export function currentSelectionConfigurationSection(
  headingName: string,
): HTMLElement {
  const heading = screen.getAllByRole("heading", { name: headingName }).at(-1);
  const section = heading?.closest("section");
  if (!section) {
    throw new Error(`expected ${headingName} configuration section`);
  }

  return section;
}

export function expectNoInlineSaveOutcomesIn(configurationRoot: HTMLElement) {
  const scoped = within(configurationRoot);
  expect(scoped.queryByText(INLINE_SAVE_SUCCESS_PATTERN)).toBeNull();
  expect(scoped.queryByText(INLINE_SAVE_FAILURE_PREFIX)).toBeNull();

  for (const status of scoped.queryAllByRole("status")) {
    const text = status.textContent ?? "";
    expect(text).not.toMatch(INLINE_SAVE_SUCCESS_PATTERN);
  }

  for (const alert of scoped.queryAllByRole("alert")) {
    const text = alert.textContent ?? "";
    expect(text).not.toMatch(INLINE_SAVE_SUCCESS_PATTERN);
    expect(text).not.toMatch(INLINE_SAVE_FAILURE_PREFIX);
    expect(text).not.toContain(WORKSTATION_STALE_VERSION_DETAIL);
  }
}

export async function expectNoSaveToastDelivery() {
  await waitFor(() => {
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
    expect(toast.warning).not.toHaveBeenCalled();
  });
}

export async function expectWorkstationSaveSuccessToast(
  workstationName = "Review",
) {
  await waitFor(() => {
    expect(toast.success).toHaveBeenCalledWith(
      "Workstation saved",
      expect.objectContaining({
        description:
          workstationDetailMessages.editableConfigurationSaveSuccess(
            workstationName,
          ),
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
  await settleCurrentSelectionEffects();
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

export async function expectResourceSaveFailedToast(errorDetail: string) {
  await waitFor(() => {
    expect(toast.error).toHaveBeenCalledWith(
      "Resource save failed",
      expect.objectContaining({
        description: expect.stringContaining(errorDetail),
        duration: PERSISTENT_TOAST_DURATION_MS,
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

export async function expectGraphDraftConflictWarningToast() {
  await waitFor(() => {
    expect(toast.warning).toHaveBeenCalledWith(
      graphDraftConflictMessages.graphDraftConflictWarningTitle,
      expect.objectContaining({
        description:
          graphDraftConflictMessages.graphDraftConflictWarningDescription,
        duration: GLOBAL_TOAST_DURATION_MS,
      }),
    );
  });
  await settleCurrentSelectionEffects();
}

export async function expectNoGraphDraftConflictWarningToast() {
  await waitFor(() => {
    const conflictCalls = (
      toast.warning as unknown as {
        mock: { calls: Array<[unknown, ...unknown[]]> };
      }
    ).mock.calls.filter(
        (call) =>
          call[0] === graphDraftConflictMessages.graphDraftConflictWarningTitle,
      );
    expect(conflictCalls).toHaveLength(0);
  });
  await settleCurrentSelectionEffects();
}
