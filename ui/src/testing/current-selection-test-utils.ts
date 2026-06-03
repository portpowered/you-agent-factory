import { act } from "@testing-library/react";

/** Flush selection-widget notification effects and store sync microtasks. */
export async function settleCurrentSelectionEffects(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}
