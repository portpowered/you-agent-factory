// @vitest-environment node

import { downloadBlobAsFile } from "./browser-download";

describe("downloadBlobAsFile outside the browser", () => {
  it("throws a clear error when document is unavailable", () => {
    expect(() =>
      downloadBlobAsFile({
        blob: new Blob(["png"], { type: "image/png" }),
        filename: "factory-aurora.png",
      }),
    ).toThrowError("Browser download is unavailable outside the document context.");
  });
});
