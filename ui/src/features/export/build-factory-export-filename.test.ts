import { describe, expect, it } from "vitest";

import { buildFactoryExportFilename } from "./build-factory-export-filename";

describe("buildFactoryExportFilename", () => {
  it("slugifies the file name", () => {
    expect(buildFactoryExportFilename(" Factory Poster 2026 ")).toBe(
      "factory-poster-2026.png",
    );
  });

  it("falls back when the name has no slug characters", () => {
    expect(buildFactoryExportFilename("!!!")).toBe("agent-factory.png");
  });
});

