// @vitest-environment node

import type { UserConfig } from "vite";
import viteConfig from "./vite.config";

const config = viteConfig as UserConfig;

describe("dashboard Vite config", () => {
  it("proxies factory events from preview to the local factory API", () => {
    expect(config.preview?.proxy?.["/events"]).toEqual(config.server?.proxy?.["/events"]);
  });

  it("keeps preview and dev proxy coverage aligned for all OpenAPI-backed API paths", () => {
    const expectedProxyPaths = [
      "/work",
      "^/factories/[^/]+/work$",
      "^/work-requests/[^/]+$",
      "^/factories/[^/]+/work-requests/[^/]+$",
      "^/work/[^/]+$",
      "^/factories/[^/]+/work/[^/]+$",
      "/events",
      "^/factories/[^/]+/events$",
      "/status",
      "^/factories/[^/]+/status$",
      "/provider-sessions/detail",
      "/factories",
      "/factory-sessions",
      "^/factory-sessions/[^/]+$",
      "/factory/~current",
      "^/factories/[^/]+/factory/~current$",
      "^/factories/[^/]+/factory/~current/editable-definition$",
      "^/factory/~current/workstations/[^/]+/prompt-template-contract$",
      "/factory/~current/editable-definition",
      "^/factory/~current/workstations/[^/]+/prompt-template-validation$",
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
