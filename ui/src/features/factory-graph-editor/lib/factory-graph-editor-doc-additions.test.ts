import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { createEmptyFactoryGraphDraft } from "./factory-graph-draft-types";
import {
  applyFactoryGraphDocAddEntityDraft,
  createFactoryGraphDocAddEntityDraft,
  validateFactoryGraphDocAddEntityDraft,
} from "./factory-graph-editor-doc-additions";

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
  workTypes: [],
};

describe("factory graph editor doc additions", () => {
  it("seeds doc add drafts with a default file name", () => {
    expect(createFactoryGraphDocAddEntityDraft(baseFactory)).toEqual({
      fileName: "new-doc.md",
      inlineContent: "",
      kind: "doc",
    });
  });

  it("rejects empty, invalid, and duplicate doc file names", () => {
    expect(
      validateFactoryGraphDocAddEntityDraft(
        { fileName: "   ", inlineContent: "", kind: "doc" },
        baseFactory,
      ),
    ).toEqual({
      fileName: "Enter a file name before adding this doc.",
    });

    expect(
      validateFactoryGraphDocAddEntityDraft(
        { fileName: "factory/docs/", inlineContent: "", kind: "doc" },
        baseFactory,
      ),
    ).toEqual({
      fileName: "Doc file names must resolve to a path under factory/docs/.",
    });

    expect(
      validateFactoryGraphDocAddEntityDraft(
        { fileName: "guide.md", inlineContent: "", kind: "doc" },
        baseFactory,
      ),
    ).toEqual({
      fileName: 'A doc at "factory/docs/guide.md" already exists in the draft.',
    });
  });

  it("applies doc additions to the graph draft", () => {
    const nextDraft = applyFactoryGraphDocAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        fileName: "notes.md",
        inlineContent: "# Notes\n",
        kind: "doc",
      },
    );

    expect(nextDraft.additions.docs).toEqual([
      {
        inlineContent: "# Notes\n",
        targetPath: "factory/docs/notes.md",
      },
    ]);
  });
});
