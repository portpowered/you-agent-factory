import { describe, expect, it } from "vitest";

import { getDocDetailMessages } from "../messages/doc-detail";
import { validateEditableDocDraft } from "./doc-editable-validation";

describe("validateEditableDocDraft", () => {
  const messages = getDocDetailMessages("en");

  it("requires a file name and inline content", () => {
    expect(
      validateEditableDocDraft(
        {
          fileName: "   ",
          inlineContent: "",
          originalExtension: ".md",
        },
        messages,
        {
          docTargetPaths: ["factory/docs/overview.md"],
          originalTargetPath: "factory/docs/overview.md",
        },
      ),
    ).toEqual({
      fileName: messages.editableConfigurationFileNameRequired,
      inlineContent: messages.editableConfigurationInlineContentRequired,
    });
  });

  it("rejects duplicate target paths", () => {
    expect(
      validateEditableDocDraft(
        {
          fileName: "guide.md",
          inlineContent: "body",
          originalExtension: ".md",
        },
        messages,
        {
          docTargetPaths: ["factory/docs/overview.md", "factory/docs/guide.md"],
          originalTargetPath: "factory/docs/overview.md",
        },
      ).fileName,
    ).toBe(messages.editableConfigurationFileNameDuplicate("guide.md"));
  });

  it("preserves the original extension during rename validation", () => {
    expect(
      validateEditableDocDraft(
        {
          fileName: "guide",
          inlineContent: "body",
          originalExtension: ".md",
        },
        messages,
        {
          docTargetPaths: ["factory/docs/overview.md"],
          originalTargetPath: "factory/docs/overview.md",
        },
      ),
    ).toEqual({});
  });
});
