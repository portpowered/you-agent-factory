import { describeWorkContentPart } from "./describe-work-content-part";

describe("describeWorkContentPart", () => {
  it("prefers a non-empty file path when present", () => {
    expect(
      describeWorkContentPart({
        type: "text",
        file: "notes.md",
      }),
    ).toBe("Text: notes.md");
  });

  it("uses label when file is absent", () => {
    expect(
      describeWorkContentPart({
        type: "json",
        label: "Payload",
      }),
    ).toBe("JSON: Payload");
  });

  it("uses content type when file and label are absent", () => {
    expect(
      describeWorkContentPart({
        type: "binary",
        contentType: "application/octet-stream",
      }),
    ).toBe("Binary (application/octet-stream)");
  });

  it("falls back to the part type label only", () => {
    expect(
      describeWorkContentPart({
        type: "image",
      }),
    ).toBe("Image");
  });
});
