import { test } from "vitest";

import { startIsolatedBrowserPreview } from "./browser-test-harness.mjs";

export const isolatedMockBrowserTest = test.extend({
  preview: async ({}, use) => {
    const preview = await startIsolatedBrowserPreview();
    try {
      await use(preview);
    } finally {
      await preview.stop();
    }
  },
});
