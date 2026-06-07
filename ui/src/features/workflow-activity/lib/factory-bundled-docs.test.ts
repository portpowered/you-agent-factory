import { describe, expect, it } from "vitest";

import {
  factoryBundledDocDisplayLabel,
  factoryBundledDocExists,
  factoryBundledDocNodeId,
  listFactoryBundledDocs,
} from "./factory-bundled-docs";

describe("factory bundled docs", () => {
  it("lists DOC bundled files under factory/docs", () => {
    const docs = listFactoryBundledDocs({
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "print('setup')" },
            targetPath: "factory/scripts/setup.py",
            type: "SCRIPT",
          },
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
        ],
      },
    });

    expect(docs).toEqual([
      {
        displayLabel: "guide.md",
        nodeId: "doc:factory/docs/guide.md",
        targetPath: "factory/docs/guide.md",
      },
      {
        displayLabel: "overview.md",
        nodeId: "doc:factory/docs/overview.md",
        targetPath: "factory/docs/overview.md",
      },
    ]);
  });

  it("derives stable node ids and display labels", () => {
    expect(factoryBundledDocNodeId("factory/docs/usage.md")).toBe(
      "doc:factory/docs/usage.md",
    );
    expect(factoryBundledDocDisplayLabel("factory/docs/usage.md")).toBe(
      "usage.md",
    );
  });

  it("checks doc existence against the current factory definition", () => {
    const factory = {
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
    };

    expect(factoryBundledDocExists(factory, "factory/docs/guide.md")).toBe(true);
    expect(factoryBundledDocExists(factory, "factory/docs/missing.md")).toBe(
      false,
    );
  });
});
