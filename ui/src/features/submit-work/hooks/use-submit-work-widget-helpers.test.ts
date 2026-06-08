import { describe, expect, it } from "vitest";

import type { SubmitWorkDraft } from "../components/submit-work-card";
import { getSubmitWorkMessages } from "../messages/submit-work";
import {
  buildStatus,
  buildStructuredSubmitItems,
  createDefaultDraft,
} from "./use-submit-work-widget-helpers";

function fileContentURL(path: string): string {
  return path.startsWith("/") ? `file://${path}` : `file:///${path}`;
}

describe("buildStructuredSubmitItems", () => {
  it("uses canonical url from staged file items instead of synthesizing staged paths", () => {
    const draft: SubmitWorkDraft = {
      items: [
        {
          id: "submission-item-1",
          text: "Review the screenshot.",
          type: "text",
        },
        {
          fileName: "ui.png",
          id: "submission-item-2",
          mediaType: "image/png",
          stagedFileRef: "staged://submit-work/ui.png",
          stagingStatus: "ready",
          type: "image",
          url: fileContentURL("/tmp/submit-work-stage/ui.png"),
        },
      ],
      requestName: "Image review",
      workTypeName: "story",
    };

    expect(buildStructuredSubmitItems(draft)).toEqual([
      {
        text: "Review the screenshot.",
        type: "text",
      },
      {
        fileName: "ui.png",
        mediaType: "image/png",
        stagedFileRef: "staged://submit-work/ui.png",
        type: "image",
        url: fileContentURL("/tmp/submit-work-stage/ui.png"),
      },
    ]);
  });

  it("omits file-backed items that are not fully staged", () => {
    const draft: SubmitWorkDraft = {
      items: [
        {
          fileName: "ui.png",
          id: "submission-item-2",
          mediaType: "image/png",
          stagedFileRef: "staged://submit-work/ui.png",
          stagingStatus: "staging",
          type: "image",
        },
      ],
      requestName: "Image review",
      workTypeName: "story",
    };

    expect(buildStructuredSubmitItems(draft)).toEqual([]);
  });
});

describe("buildStatus", () => {
  const messages = getSubmitWorkMessages("en");

  it("keeps missing request name feedback field-scoped after a submit attempt", () => {
    const draft = {
      ...createDefaultDraft(),
      workTypeName: "story",
    };

    expect(
      buildStatus({
        draft,
        error: null,
        isSubmitting: false,
        isSuccess: false,
        messages,
        showValidation: true,
        submitWorkTypeNames: ["story"],
      }),
    ).toEqual({
      kind: "guidance",
      message: messages.statusMessages.ready,
    });
  });

  it("still surfaces detached validation for non-request-name field issues", () => {
    const draft = {
      ...createDefaultDraft(),
      requestName: "Driver review",
    };

    expect(
      buildStatus({
        draft,
        error: null,
        isSubmitting: false,
        isSuccess: false,
        messages,
        showValidation: true,
        submitWorkTypeNames: ["story"],
      }),
    ).toEqual({
      kind: "validation-error",
      message: messages.validationMessages.workTypeRequired,
    });
  });
});
