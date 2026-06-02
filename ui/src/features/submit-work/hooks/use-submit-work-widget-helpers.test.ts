import { describe, expect, it } from "vitest";

import type { SubmitWorkDraft } from "../components/submit-work-card";
import { buildStructuredSubmitItems } from "./use-submit-work-widget-helpers";

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
