import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { buildDraftAppliedFactoryDefinition } from "./draft/factory-graph-draft-apply";
import { createEmptyFactoryGraphDraft } from "./draft/factory-graph-draft-types";
import {
  applyFactoryGraphAddEntityDraft,
  validateFactoryGraphAddEntityDraft,
} from "./editor/factory-graph-editor-additions";
import {
  applyFactoryGraphDocRemoval,
  buildFactoryGraphDocRemovalIntent,
  factoryGraphDocNodeIdForTargetPath,
  suggestDefaultDocFileName,
} from "./factory-graph-doc-editor";

const baseFactory: CanonicalFactoryDefinition = {
  name: "Current Factory",
  supportingFiles: {
    bundledFiles: [
      {
        content: { encoding: "utf-8", inline: "# Guide\n" },
        targetPath: "factory/docs/guide.md",
        type: "DOC",
      },
    ],
  },
  version: { logical: "1", physical: "2026-06-08T00:00:00Z" },
  workTypes: [],
};

describe("factory graph doc editor", () => {
  it("suggests unique default doc file names", () => {
    expect(suggestDefaultDocFileName(baseFactory)).toBe("new-doc.md");
    expect(
      suggestDefaultDocFileName(
        buildDraftAppliedFactoryDefinition(
          baseFactory,
          applyFactoryGraphAddEntityDraft(createEmptyFactoryGraphDraft(), {
            fileName: "new-doc.md",
            inlineContent: "",
            kind: "doc",
          }),
        ),
      ),
    ).toBe("new-doc-2.md");
    expect(
      suggestDefaultDocFileName(
        buildDraftAppliedFactoryDefinition(
          baseFactory,
          applyFactoryGraphAddEntityDraft(
            applyFactoryGraphAddEntityDraft(createEmptyFactoryGraphDraft(), {
              fileName: "new-doc.md",
              inlineContent: "",
              kind: "doc",
            }),
            {
              fileName: "new-doc-2.md",
              inlineContent: "",
              kind: "doc",
            },
          ),
        ),
      ),
    ).toBe("new-doc-3.md");
  });

  it("validates and applies doc additions to the graph draft", () => {
    const draft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        fileName: "notes.md",
        inlineContent: "# Notes\n",
        kind: "doc",
      },
    );

    expect(draft.additions.docs).toEqual([
      {
        inlineContent: "# Notes\n",
        targetPath: "factory/docs/notes.md",
      },
    ]);
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          fileName: "guide.md",
          inlineContent: "",
          kind: "doc",
        },
        baseFactory,
      ),
    ).toEqual({
      fileName: 'A doc at "factory/docs/guide.md" already exists in the draft.',
    });
  });

  it("returns null removal intents for invalid or missing doc nodes", () => {
    expect(
      buildFactoryGraphDocRemovalIntent({
        baseFactoryDefinition: baseFactory,
        draft: createEmptyFactoryGraphDraft(),
        nodeId: "workstation:draft",
      }),
    ).toBeNull();

    expect(
      buildFactoryGraphDocRemovalIntent({
        baseFactoryDefinition: baseFactory,
        draft: createEmptyFactoryGraphDraft(),
        nodeId: "doc:factory/docs/missing.md",
      }),
    ).toBeNull();
  });

  it("builds destructive doc removal intents and applies removals", () => {
    const intent = buildFactoryGraphDocRemovalIntent({
      baseFactoryDefinition: baseFactory,
      draft: createEmptyFactoryGraphDraft(),
      nodeId: "doc:factory/docs/guide.md",
    });

    expect(intent).toMatchObject({
      requiresConfirmation: true,
      targetPath: "factory/docs/guide.md",
      title: "Remove guide.md doc?",
    });

    const nextDraft = applyFactoryGraphDocRemoval(
      createEmptyFactoryGraphDraft(),
      baseFactory,
      "factory/docs/guide.md",
    );
    const pending = buildDraftAppliedFactoryDefinition(baseFactory, nextDraft);

    expect(pending.supportingFiles?.bundledFiles).toEqual([]);
  });

  it("removes draft-only docs without recording a persisted removal", () => {
    const draft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        fileName: "draft-only.md",
        inlineContent: "# Draft\n",
        kind: "doc",
      },
    );

    const nextDraft = applyFactoryGraphDocRemoval(
      draft,
      baseFactory,
      "factory/docs/draft-only.md",
    );

    expect(nextDraft.additions.docs).toEqual([]);
    expect(nextDraft.removals.docs).toEqual([]);
  });

  it("builds canonical doc node ids for target paths", () => {
    expect(factoryGraphDocNodeIdForTargetPath("factory/docs/guide.md")).toBe(
      "doc:factory/docs/guide.md",
    );
  });
});
