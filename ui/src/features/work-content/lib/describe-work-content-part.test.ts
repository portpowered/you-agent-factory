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

  it("derives a readable label from file-backed content urls", () => {
    expect(
      describeWorkContentPart({
        type: "image",
        url: "file://screenshot.png",
      }),
    ).toBe("Image: screenshot.png");
  });

  it("derives a readable label from absolute file urls", () => {
    expect(
      describeWorkContentPart({
        type: "image",
        url: "file:///tmp/assets/diagram.png",
      }),
    ).toBe("Image: diagram.png");
  });

  it("uses the file url host when the path is host-only", () => {
    expect(
      describeWorkContentPart({
        type: "image",
        url: "file://fixtures",
      }),
    ).toBe("Image: fixtures");
  });

  it("derives a readable label from remote content urls", () => {
    expect(
      describeWorkContentPart({
        type: "image",
        url: "https://cdn.example.com/assets/hero.png",
      }),
    ).toBe("Image: hero.png");
  });

  it("uses the remote host when the url has no path segments", () => {
    expect(
      describeWorkContentPart({
        type: "audio",
        url: "https://audio.example.com",
      }),
    ).toBe("Audio: audio.example.com");
  });

  it("falls back to the full url when parsing fails", () => {
    expect(
      describeWorkContentPart({
        type: "image",
        url: "not-a-valid-url",
      }),
    ).toBe("Image: not-a-valid-url");
  });

  it("uses label when file is absent", () => {
    expect(
      describeWorkContentPart({
        type: "json",
        label: "Payload",
      }),
    ).toBe("JSON: Payload");
  });

  it("skips empty file strings and prefers label", () => {
    expect(
      describeWorkContentPart({
        type: "text",
        file: "",
        label: "Inline notes",
      }),
    ).toBe("Text: Inline notes");
  });

  it("skips empty labels and uses content type", () => {
    expect(
      describeWorkContentPart({
        type: "json",
        label: "",
        contentType: "application/json",
      }),
    ).toBe("JSON (application/json)");
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
