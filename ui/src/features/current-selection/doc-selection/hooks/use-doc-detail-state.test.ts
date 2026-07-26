import { describe, expect, it } from "vitest";

import { deriveDocDetailState } from "./use-doc-detail-state";

describe("deriveDocDetailState", () => {
  it("loads nested doc text from the pending factory", () => {
    const targetPath = "factory/docs/standards/review.md";
    expect(
      deriveDocDetailState({
        pendingFactoryDefinition: {
          name: "Current Factory",
          supportingFiles: {
            bundledFiles: [
              {
                content: { encoding: "utf-8", inline: "# Review standards\n" },
                targetPath,
                type: "DOC",
              },
            ],
          },
        },
        targetPath,
      }),
    ).toEqual({
      status: "ready",
      displayLabel: "review.md",
      inlineContent: "# Review standards\n",
      targetPath,
    });
  });

  it("reports empty when neither saved nor pending content exists", () => {
    expect(
      deriveDocDetailState({
        pendingFactoryDefinition: null,
        targetPath: "factory/docs/guide.md",
      }),
    ).toEqual({ status: "empty" });
  });

  it("prefers saved event content over the pending factory", () => {
    const targetPath = "factory/docs/guide.md";
    expect(
      deriveDocDetailState({
        pendingFactoryDefinition: {
          name: "Current Factory",
          supportingFiles: {
            bundledFiles: [
              {
                content: { encoding: "utf-8", inline: "# Pending\n" },
                targetPath,
                type: "DOC",
              },
            ],
          },
        },
        savedBundledDoc: {
          content: { encoding: "utf-8", inline: "# Saved\n" },
          targetPath,
          type: "DOC",
        },
        targetPath,
      }),
    ).toEqual({
      status: "ready",
      displayLabel: "guide.md",
      inlineContent: "# Saved\n",
      targetPath,
    });
  });
});
