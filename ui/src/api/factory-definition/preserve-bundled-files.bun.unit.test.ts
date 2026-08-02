import { describe, expect, it } from "bun:test";

import { preserveExistingBundledFilesWhenAbsent } from "./preserve-bundled-files";

describe("preserveExistingBundledFilesWhenAbsent", () => {
  it("preserves bundled files from the saved document when the incoming factory omits them", () => {
    const existing = {
      name: "factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
        ],
      },
    };
    const incoming = {
      name: "factory",
      workers: [{ name: "writer", type: "MODEL_WORKER" as const }],
    };

    expect(preserveExistingBundledFilesWhenAbsent(incoming, existing)).toEqual({
      ...incoming,
      supportingFiles: existing.supportingFiles,
    });
  });

  it("keeps explicit incoming bundled files when the event payload includes them", () => {
    const existing = {
      name: "factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Stale" },
            targetPath: "factory/docs/stale.md",
            type: "DOC",
          },
        ],
      },
    };
    const incoming = {
      name: "factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Current" },
            targetPath: "factory/docs/current.md",
            type: "DOC",
          },
        ],
      },
    };

    expect(preserveExistingBundledFilesWhenAbsent(incoming, existing)).toEqual(
      incoming,
    );
  });

  it("keeps explicit empty bundled files when the incoming factory clears docs", () => {
    const existing = {
      name: "factory",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Stale" },
            targetPath: "factory/docs/stale.md",
            type: "DOC",
          },
        ],
      },
    };
    const incoming = {
      name: "factory",
      supportingFiles: {
        bundledFiles: [],
      },
    };

    expect(preserveExistingBundledFilesWhenAbsent(incoming, existing)).toEqual(
      incoming,
    );
  });
});
