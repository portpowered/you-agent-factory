import { act, waitFor } from "@testing-library/react";

/** Flush selection-widget notification effects and store sync microtasks. */
export async function settleCurrentSelectionEffects(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}

/** Like `waitFor`, then flush selection-related async React updates before assertions end. */
export async function waitForCurrentSelection(
  callback: () => void | Promise<void>,
  options?: Parameters<typeof waitFor>[1],
): Promise<void> {
  await waitFor(callback, options);
  await settleCurrentSelectionEffects();
}
