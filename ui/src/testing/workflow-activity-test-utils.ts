import { act } from "@testing-library/react";

/** Flush workflow-activity graph card editor and view-model async effects. */
export async function settleWorkflowActivityGraphEffects(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}
