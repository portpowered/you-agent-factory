import { afterAll, test } from "vitest";

import {
  openBrowserPage as openUnscopedBrowserPage,
  startDedicatedBrowserPreview,
  startIsolatedBrowserPreview,
  stopBrowserPreview,
} from "./browser-test-harness.mjs";

afterAll(async () => {
  await stopBrowserPreview();
});

function isolatedBrowserScenarioTest(
  name,
  scenario,
  timeout,
  { injectRuntimeAPIOrigin, startPreview },
) {
  return test.concurrent(
    name,
    async ({ expect }) => {
      const preview = await startPreview();
      try {
        await scenario({
          expect,
          openBrowserPage: (options = {}) => {
            const scopedOptions = { ...options };
            if (injectRuntimeAPIOrigin) {
              scopedOptions.apiOrigin = preview.apiOrigin;
            }
            return openUnscopedBrowserPage(scopedOptions);
          },
          preview,
        });
      } finally {
        await preview.stop();
      }
    },
    timeout,
  );
}

export function isolatedMockBrowserTest(name, scenario, timeout) {
  return isolatedBrowserScenarioTest(name, scenario, timeout, {
    injectRuntimeAPIOrigin: true,
    startPreview: startIsolatedBrowserPreview,
  });
}

export function isolatedBrowserTest(name, scenario, timeout) {
  return isolatedBrowserScenarioTest(name, scenario, timeout, {
    injectRuntimeAPIOrigin: false,
    startPreview: startDedicatedBrowserPreview,
  });
}
