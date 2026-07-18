import { describe, expect, it } from "vitest";

import { verifyInstalledConsumer } from "../scripts/verify-installed-consumer.mjs";

describe("installed components consumer", () => {
  it("typechecks, bundles, and renders the registry tarball in a clean application", async () => {
    await expect(verifyInstalledConsumer()).resolves.toEqual({
      packageName: "@you-agent-factory/components",
      viewports: ["mobile", "desktop"],
    });
  }, 180_000);
});
