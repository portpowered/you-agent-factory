import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { buildDraftAppliedFactoryDefinition } from "./draft/factory-graph-draft-apply";
import { createEmptyFactoryGraphDraft } from "./draft/factory-graph-draft-types";
import { applyFactoryGraphAddEntityDraft } from "./editor/factory-graph-editor-additions";

const baseFactory: CanonicalFactoryDefinition = {
  name: "Current Factory",
  supportingFiles: {
    bundledFiles: [
      {
        content: { encoding: "utf-8", inline: "print('setup')\n" },
        targetPath: "factory/scripts/setup.py",
        type: "SCRIPT",
      },
    ],
  },
  version: { logical: "1", physical: "2026-06-08T00:00:00Z" },
  workTypes: [],
};

describe("buildDraftAppliedFactoryDefinition docs", () => {
  it("adds bundled docs without disturbing unrelated bundled files", () => {
    const draft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        fileName: "overview.md",
        inlineContent: "# Overview\n",
        kind: "doc",
      },
    );

    expect(
      buildDraftAppliedFactoryDefinition(baseFactory, draft).supportingFiles
        ?.bundledFiles,
    ).toEqual([
      {
        content: { encoding: "utf-8", inline: "print('setup')\n" },
        targetPath: "factory/scripts/setup.py",
        type: "SCRIPT",
      },
      {
        content: { encoding: "utf-8", inline: "# Overview\n" },
        targetPath: "factory/docs/overview.md",
        type: "DOC",
      },
    ]);
  });
});
