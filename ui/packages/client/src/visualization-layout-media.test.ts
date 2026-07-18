import { describe, expect, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import customerSupportLayout from "../examples/customer-support.factory-visualization-layout.v1.json";
import {
  type FactoryDefinition,
  safeParseFactoryVisualizationLayout,
} from "./index.js";

const imageSignatures = {
  "image/png": [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a],
  "image/jpeg": [0xff, 0xd8, 0xff],
  "image/webp": [0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50],
} as const;

function exampleFactory(): FactoryDefinition {
  return structuredClone(customerSupportRecording.factory) as FactoryDefinition;
}

function exampleLayout(): Record<string, unknown> {
  return structuredClone(customerSupportLayout) as Record<string, unknown>;
}

function embeddedImageBase64(
  mediaType: keyof typeof imageSignatures,
  byteLength = imageSignatures[mediaType].length,
): string {
  const bytes = new Uint8Array(byteLength);
  bytes.set(imageSignatures[mediaType]);
  return Buffer.from(bytes).toString("base64");
}

function imageAnnotation(
  id: string,
  mediaType: keyof typeof imageSignatures,
  base64 = embeddedImageBase64(mediaType),
): Record<string, unknown> {
  return {
    id,
    kind: "image",
    position: { x: 0, y: 0 },
    size: { width: 1, height: 1 },
    altText: `${id} alternative`,
    source: { kind: "embedded", mediaType, base64 },
  };
}

describe("Factory visualization supported embedded rasters", () => {
  it.each(["image/png", "image/jpeg", "image/webp"] as const)(
    "accepts a valid embedded %s signature",
    (mediaType) => {
      const input = exampleLayout();
      input.annotations = [imageAnnotation("supported-image", mediaType)];
      input.nodeEmptyStates = [];
      expect(
        safeParseFactoryVisualizationLayout(input, exampleFactory()),
      ).toMatchObject({ success: true });
    },
  );
});

describe("Factory visualization embedded raster diagnostics", () => {
  it.each([
    ["whitespace", "iVBORw0K Ggo="],
    ["data URL", "data:image/png;base64,iVBORw0KGgo="],
    ["invalid alphabet", "iVBORw0KGg*="],
    ["invalid padding", "iVBORw0KGgo==="],
    ["unpadded encoding", "iVBORw0KGgo"],
    ["non-canonical encoding", "Zh=="],
  ])("rejects %s in base64", (_, base64) => {
    const input = exampleLayout();
    input.annotations = [
      imageAnnotation("invalid-base64", "image/png", base64),
    ];
    input.nodeEmptyStates = [];
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code: "invalid_base64",
          path: ["annotations", 0, "source", "base64"],
        }),
      );
    }
  });

  it("rejects empty payloads, unsupported media, and signature mismatches", () => {
    const input = exampleLayout();
    input.annotations = [
      imageAnnotation("empty", "image/png", ""),
      imageAnnotation(
        "mismatch",
        "image/jpeg",
        embeddedImageBase64("image/png"),
      ),
      {
        ...imageAnnotation("unsupported", "image/png"),
        source: {
          kind: "embedded",
          mediaType: "image/gif",
          base64: "R0lGODlh",
        },
      },
    ];
    input.nodeEmptyStates = [];
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "empty_image_payload",
            path: ["annotations", 0, "source", "base64"],
          }),
          expect.objectContaining({
            code: "image_media_type_mismatch",
            path: ["annotations", 1, "source", "base64"],
          }),
          expect.objectContaining({
            code: "unsupported_image_media_type",
            path: ["annotations", 2, "source", "mediaType"],
          }),
        ]),
      );
    }
  });

  it("requires non-blank alternative text of at most 500 characters", () => {
    const missing = imageAnnotation("missing-alt", "image/png");
    delete missing.altText;
    const blank = imageAnnotation("blank-alt", "image/png");
    blank.altText = "  ";
    const overlong = imageAnnotation("long-alt", "image/png");
    overlong.altText = "a".repeat(501);
    const input = exampleLayout();
    input.annotations = [missing, blank, overlong];
    input.nodeEmptyStates = [];
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "missing_required_field",
            path: ["annotations", 0, "altText"],
          }),
          expect.objectContaining({
            code: "empty_text",
            path: ["annotations", 1, "altText"],
          }),
          expect.objectContaining({
            code: "text_too_long",
            path: ["annotations", 2, "altText"],
          }),
        ]),
      );
    }
  });
});

describe("Factory visualization embedded raster byte limits", () => {
  it("accepts 2 MiB and rejects the image occurrence one byte over", () => {
    const input = exampleLayout();
    input.annotations = [
      imageAnnotation(
        "at-limit",
        "image/png",
        embeddedImageBase64("image/png", 2 * 1024 * 1024),
      ),
      imageAnnotation(
        "over-limit",
        "image/png",
        embeddedImageBase64("image/png", 2 * 1024 * 1024 + 1),
      ),
    ];
    input.nodeEmptyStates = [];
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(
        result.issues.filter(({ code }) => code === "image_too_large"),
      ).toEqual([
        expect.objectContaining({
          path: ["annotations", 1, "source", "base64"],
        }),
      ]);
    }
  });

  it("identifies the image occurrence that crosses the 8 MiB aggregate limit", () => {
    const twoMiB = embeddedImageBase64("image/png", 2 * 1024 * 1024);
    const input = exampleLayout();
    input.annotations = [0, 1, 2, 3].map((index) =>
      imageAnnotation(`aggregate-${index}`, "image/png", twoMiB),
    );
    input.nodeEmptyStates = [
      {
        nodeId: "workstation:triage",
        content: {
          kind: "image",
          altText: "Aggregate-crossing occurrence",
          source: { kind: "embedded", mediaType: "image/png", base64: twoMiB },
        },
      },
    ];
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(
        result.issues.filter(
          ({ code }) => code === "aggregate_image_bytes_exceeded",
        ),
      ).toEqual([
        expect.objectContaining({
          path: ["nodeEmptyStates", 0, "content", "source", "base64"],
        }),
      ]);
    }
  });
});
