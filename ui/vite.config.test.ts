// @vitest-environment node

import type { UserConfig } from "vite";
import viteConfig, { isDashboardUnitVitestRun } from "./vite.config";

const config = viteConfig as UserConfig;

describe("dashboard Vite config", () => {
  it("dedupes context-bearing packages used by linked component sources", () => {
    expect(config.resolve?.dedupe).toEqual(
      expect.arrayContaining([
        "@radix-ui/react-collapsible",
        "@radix-ui/react-compose-refs",
        "@radix-ui/react-dialog",
        "@radix-ui/react-popover",
        "@radix-ui/react-scroll-area",
        "@radix-ui/react-select",
        "@radix-ui/react-slot",
        "react",
        "react-dom",
        "@xyflow/react",
        "@xyflow/system",
        "react-redux",
        "recharts",
      ]),
    );
  });

  it("recognizes only an explicit isolated dashboard-unit invocation", () => {
    expect(
      isDashboardUnitVitestRun(["node", "vitest", "--project=dashboard-unit"], {
        VITEST: "true",
      }),
    ).toBe(true);
    expect(
      isDashboardUnitVitestRun(
        ["node", "vitest", "run", "--project", "dashboard-unit"],
        { VITEST: "true" },
      ),
    ).toBe(true);
    expect(
      isDashboardUnitVitestRun(
        ["node", "vitest", "--project=dashboard-component"],
        {
          VITEST: "true",
        },
      ),
    ).toBe(false);
  });

  it("keeps preview and dev proxy coverage aligned for all OpenAPI-backed API paths", () => {
    const expectedProxyPaths = [
      "/work",
      "^/factory-sessions/[^/]+/work$",
      "^/factory-sessions/[^/]+/invocations$",
      "^/work-requests/[^/]+$",
      "^/factory-sessions/[^/]+/work-requests/[^/]+$",
      "^/work/[^/]+$",
      "^/factory-sessions/[^/]+/work/[^/]+$",
      "^/factory-sessions/[^/]+/events$",
      "/status",
      "^/factory-sessions/[^/]+/status$",
      "^/factory-sessions/[^/]+/sync-preflight$",
      "^/factory-sessions/[^/]+/dispatches$",
      "^/factory-sessions/[^/]+/dispatches/[^/]+$",
      "^/factory-sessions/[^/]+/artifacts$",
      "^/factory-sessions/[^/]+/artifacts/[^/]+$",
      "/provider-sessions/detail",
      "/factories",
      "/factory-sessions",
      "^/factory-sessions/[^/]+$",
      "/factory-sessions/~default/factory",
      "^/factory-sessions/[^/]+/factory$",
      "^/factory-sessions/[^/]+/factory/workstations/[^/]+/prompt-template-contract$",
      "^/factory-sessions/[^/]+/factory/workstations/[^/]+/prompt-template-validation$",
    ] as const;

    expect(Object.keys(config.server?.proxy ?? {})).toEqual(expectedProxyPaths);
    expect(config.preview?.proxy).toEqual(config.server?.proxy);

    for (const path of expectedProxyPaths) {
      expect(config.server?.proxy?.[path]).toEqual({
        target: "http://127.0.0.1:7437",
        changeOrigin: true,
      });
    }
  });
});
