// @vitest-environment node

import { describe, expect, it, vi } from "vitest";

import { runSharedBrowserBuild } from "./browser-build-lock.mjs";

describe("shared browser build lock", () => {
  it("prebuilt previews consume ready output and never rebuild missing output", async () => {
    const build = vi.fn();
    for (const ready of [true, false]) {
      const running = runSharedBrowserBuild({
        build,
        buildState: {},
        buildCacheKey: "prebuilt",
        ready: async () => ready,
        prebuilt: true,
      });
      if (ready) await expect(running).resolves.toBeUndefined();
      else await expect(running).rejects.toThrow(/missing or invalid/);
    }
    expect(build).not.toHaveBeenCalled();
  });

  it("shares one pending build across concurrent preview setup", async () => {
    const buildState = {};
    let releaseBuild;
    const build = vi.fn(
      () =>
        new Promise((resolve) => {
          releaseBuild = resolve;
        }),
    );
    const ready = vi.fn().mockResolvedValue(false);
    const options = {
      build,
      buildCacheKey: "browser-build",
      buildState,
      ready,
    };

    const builds = [
      runSharedBrowserBuild(options),
      runSharedBrowserBuild(options),
      runSharedBrowserBuild(options),
    ];
    await vi.waitFor(() => expect(build).toHaveBeenCalledOnce());
    releaseBuild();
    await Promise.all(builds);

    expect(ready).toHaveBeenCalledOnce();
    expect(build).toHaveBeenCalledOnce();
  });

  it("clears a failed build so a later preview can retry", async () => {
    const buildState = {};
    const build = vi
      .fn()
      .mockRejectedValueOnce(new Error("build failed"))
      .mockResolvedValueOnce(undefined);
    const options = {
      build,
      buildCacheKey: "browser-build",
      buildState,
      ready: vi.fn().mockResolvedValue(false),
    };

    await expect(runSharedBrowserBuild(options)).rejects.toThrow(
      "build failed",
    );
    await expect(runSharedBrowserBuild(options)).resolves.toBeUndefined();

    expect(build).toHaveBeenCalledTimes(2);
  });
});
