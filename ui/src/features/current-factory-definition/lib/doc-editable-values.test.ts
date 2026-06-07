import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableDocDraft,
  buildDocTargetPathFromFileName,
  editableDocDraftFromValues,
  resolveDocTargetPathFromDraft,
  resolveEditableDocValues,
  resolveFileNameWithExtensionPreserved,
} from "./doc-editable-values";

function factoryWithBundledFiles(
  bundledFiles: CanonicalFactoryDefinition["supportingFiles"],
): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    supportingFiles: bundledFiles,
    version: { logical: "1", physical: "2026-06-08T00:00:00Z" },
    workTypes: [],
  };
}

describe("doc-editable-values", () => {
  it("resolves editable values from bundled DOC entries", () => {
    const factory = factoryWithBundledFiles({
      bundledFiles: [
        {
          content: { encoding: "utf-8", inline: "# Overview\n" },
          targetPath: "factory/docs/overview.md",
          type: "DOC",
        },
      ],
    });

    expect(resolveEditableDocValues(factory, "factory/docs/overview.md")).toEqual(
      {
        fileName: "overview.md",
        inlineContent: "# Overview\n",
        targetPath: "factory/docs/overview.md",
      },
    );
  });

  it("preserves the original extension when the rename omits one", () => {
    expect(
      resolveFileNameWithExtensionPreserved("guide", ".md"),
    ).toBe("guide.md");
    expect(resolveDocTargetPathFromDraft({
      fileName: "guide",
      inlineContent: "body",
      originalExtension: ".md",
    })).toBe("factory/docs/guide.md");
  });

  it("keeps an explicitly changed extension", () => {
    expect(
      resolveFileNameWithExtensionPreserved("guide.txt", ".md"),
    ).toBe("guide.txt");
  });

  it("applies draft edits to the selected bundled doc only", () => {
    const factory = factoryWithBundledFiles({
      bundledFiles: [
        {
          content: { encoding: "utf-8", inline: "# Overview\n" },
          targetPath: "factory/docs/overview.md",
          type: "DOC",
        },
        {
          content: { encoding: "utf-8", inline: "print('setup')\n" },
          targetPath: "factory/scripts/setup-workspace.py",
          type: "SCRIPT",
        },
      ],
    });

    const pending = applyEditableDocDraft(
      factory,
      "factory/docs/overview.md",
      {
        fileName: "guide.md",
        inlineContent: "# Guide\n",
        originalExtension: ".md",
      },
    );

    expect(pending?.supportingFiles?.bundledFiles).toEqual([
      {
        content: { encoding: "utf-8", inline: "# Guide\n" },
        targetPath: "factory/docs/guide.md",
        type: "DOC",
      },
      {
        content: { encoding: "utf-8", inline: "print('setup')\n" },
        targetPath: "factory/scripts/setup-workspace.py",
        type: "SCRIPT",
      },
    ]);
  });

  it("builds canonical target paths from file names", () => {
    expect(buildDocTargetPathFromFileName("notes.md")).toBe(
      "factory/docs/notes.md",
    );
    expect(
      editableDocDraftFromValues({
        fileName: "notes.md",
        inlineContent: "hello",
        targetPath: "factory/docs/notes.md",
      }).originalExtension,
    ).toBe(".md");
  });
});
