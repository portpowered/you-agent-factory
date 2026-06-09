import { describe, expect, it } from "vitest";

import { getDocDetailMessages } from "../messages/doc-detail";
import {
  formatEditableDocOverwriteFieldLabels,
  resolveEditableDocOverwriteFields,
} from "./editable-doc-overwrite-fields";

describe("editable doc overwrite fields", () => {
  it("detects file name and inline content drift from the session start", () => {
    expect(
      resolveEditableDocOverwriteFields(
        {
          fileName: "guide.md",
          inlineContent: "# Guide\n",
          originalExtension: ".md",
        },
        {
          fileName: "notes.md",
          inlineContent: "# Notes\n",
          originalExtension: ".md",
        },
        {
          fileName: "overview.md",
          inlineContent: "# Overview\n",
          originalExtension: ".md",
        },
      ),
    ).toEqual(["fileName", "inlineContent"]);
  });

  it("formats overwrite field labels for warnings", () => {
    const messages = getDocDetailMessages("en");

    expect(
      formatEditableDocOverwriteFieldLabels(["fileName", "inlineContent"], {
        editableConfigurationFileNameFieldLabel:
          messages.editableConfigurationFileNameFieldLabel,
        editableConfigurationInlineContentFieldLabel:
          messages.editableConfigurationInlineContentFieldLabel,
      }),
    ).toBe("file name, doc content");
  });
});
