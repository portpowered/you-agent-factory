import { describe, expect, it } from "vitest";

import {
  factoryBundledDocDisplayLabel,
  factoryBundledDocExists,
  factoryBundledDocNodeId,
  isFactoryBundledDocTargetPath,
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

  it("accepts only non-empty paths under factory/docs", () => {
    expect(isFactoryBundledDocTargetPath("factory/docs/guide.md")).toBe(true);
    expect(isFactoryBundledDocTargetPath("factory/docs/")).toBe(false);
    expect(isFactoryBundledDocTargetPath("factory/scripts/setup.py")).toBe(
      false,
    );
  });

  it("derives stable node ids and display labels", () => {
    expect(factoryBundledDocNodeId("factory/docs/usage.md")).toBe(
      "doc:factory/docs/usage.md",
    );
    expect(factoryBundledDocDisplayLabel("factory/docs/usage.md")).toBe(
      "usage.md",
    );
  });

  it("skips DOC bundled files with invalid or empty target paths", () => {
    expect(
      listFactoryBundledDocs({
        supportingFiles: {
          bundledFiles: [
            {
              content: { encoding: "utf-8", inline: "# Missing path" },
              targetPath: "   ",
              type: "DOC",
            },
            {
              content: { encoding: "utf-8", inline: "# Outside docs" },
              targetPath: "factory/scripts/setup.py",
              type: "DOC",
            },
            {
              content: { encoding: "utf-8", inline: "# Valid" },
              targetPath: "factory/docs/valid.md",
              type: "DOC",
            },
          ],
        },
      }),
    ).toEqual([
      {
        displayLabel: "valid.md",
        nodeId: "doc:factory/docs/valid.md",
        targetPath: "factory/docs/valid.md",
      },
    ]);
  });

  it("lists nested DOC bundled files under factory/docs subdirectories", () => {
    const docs = listFactoryBundledDocs({
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "# Review standards" },
            targetPath: "factory/docs/standards/review.md",
            type: "DOC",
          },
        ],
      },
    });

    expect(docs).toEqual([
      {
        displayLabel: "overview.md",
        nodeId: "doc:factory/docs/overview.md",
        targetPath: "factory/docs/overview.md",
      },
      {
        displayLabel: "review.md",
        nodeId: "doc:factory/docs/standards/review.md",
        targetPath: "factory/docs/standards/review.md",
      },
    ]);
    expect(
      isFactoryBundledDocTargetPath("factory/docs/standards/review.md"),
    ).toBe(true);
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

    expect(factoryBundledDocExists(factory, "factory/docs/guide.md")).toBe(
      true,
    );
    expect(factoryBundledDocExists(factory, "factory/docs/missing.md")).toBe(
      false,
    );
  });
});
