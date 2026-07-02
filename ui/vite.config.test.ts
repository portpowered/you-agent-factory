// @vitest-environment node

import type { UserConfig } from "vite";
import viteConfig from "./vite.config";

const config = viteConfig as UserConfig;

describe("dashboard Vite config", () => {
  it("proxies factory events from preview to the local factory API", () => {
    expect(config.preview?.proxy?.["/events"]).toEqual(
      config.server?.proxy?.["/events"],
    );
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
      "/events",
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
