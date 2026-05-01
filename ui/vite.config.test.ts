// @vitest-environment node

import type { UserConfig } from "vite";
import viteConfig from "./vite.config";

const config = viteConfig as UserConfig;

describe("dashboard Vite config", () => {
  it("proxies factory events from preview to the local factory API", () => {
    expect(config.preview?.proxy?.["/events"]).toEqual(config.server?.proxy?.["/events"]);
  });
});
