import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

type InputPath = readonly (string | number)[];

export const MAX_EMBEDDED_IMAGE_BYTES = 2 * 1024 * 1024;
export const MAX_LAYOUT_IMAGE_BYTES = 8 * 1024 * 1024;
export const MAX_IMAGE_ALT_TEXT_LENGTH = 500;

const canonicalPaddedBase64 =
  /^(?:[A-Za-z\d+/]{4})*(?:[A-Za-z\d+/]{2}==|[A-Za-z\d+/]{3}=)?$/u;
const base64Alphabet =
  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

export interface ImageByteBudget {
  total: number;
  aggregateLimitReported: boolean;
}

function decodedByteLength(value: string): number {
  const padding = value.endsWith("==") ? 2 : value.endsWith("=") ? 1 : 0;
  return (value.length / 4) * 3 - padding;
}

function isCanonicalBase64(value: string): boolean {
  if (!canonicalPaddedBase64.test(value)) return false;
  if (value.endsWith("==")) {
    return (base64Alphabet.indexOf(value[value.length - 3] ?? "") & 0x0f) === 0;
  }
  if (value.endsWith("=")) {
    return (base64Alphabet.indexOf(value[value.length - 2] ?? "") & 0x03) === 0;
  }
  return true;
}

function decodePrefix(value: string, byteLimit: number): Uint8Array {
  const decoded = new Uint8Array(Math.min(decodedByteLength(value), byteLimit));
  let decodedIndex = 0;
  for (
    let index = 0;
    index < value.length && decodedIndex < byteLimit;
    index += 4
  ) {
    const first = base64Alphabet.indexOf(value[index] ?? "");
    const second = base64Alphabet.indexOf(value[index + 1] ?? "");
    const thirdCharacter = value[index + 2] ?? "=";
    const fourthCharacter = value[index + 3] ?? "=";
    const third =
      thirdCharacter === "=" ? 0 : base64Alphabet.indexOf(thirdCharacter);
    const fourth =
      fourthCharacter === "=" ? 0 : base64Alphabet.indexOf(fourthCharacter);
    const bits = (first << 18) | (second << 12) | (third << 6) | fourth;
    decoded[decodedIndex] = (bits >> 16) & 0xff;
    decodedIndex += 1;
    if (thirdCharacter !== "=" && decodedIndex < decoded.length) {
      decoded[decodedIndex] = (bits >> 8) & 0xff;
      decodedIndex += 1;
    }
    if (fourthCharacter !== "=" && decodedIndex < decoded.length) {
      decoded[decodedIndex] = bits & 0xff;
      decodedIndex += 1;
    }
  }
  return decoded;
}

function startsWith(bytes: Uint8Array, signature: readonly number[]): boolean {
  return (
    bytes.length >= signature.length &&
    signature.every((byte, index) => bytes[index] === byte)
  );
}

function signatureMatches(mediaType: string, bytes: Uint8Array): boolean {
  if (mediaType === "image/png") {
    return startsWith(bytes, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  }
  if (mediaType === "image/jpeg") {
    return startsWith(bytes, [0xff, 0xd8, 0xff]);
  }
  return (
    mediaType === "image/webp" &&
    startsWith(bytes, [0x52, 0x49, 0x46, 0x46]) &&
    bytes.length >= 12 &&
    bytes[8] === 0x57 &&
    bytes[9] === 0x45 &&
    bytes[10] === 0x42 &&
    bytes[11] === 0x50
  );
}

export function validateEmbeddedImageData(
  base64: string,
  mediaType: string,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
  budget: ImageByteBudget,
): void {
  if (!isCanonicalBase64(base64)) {
    issues.push({
      category: "semantic",
      code: "invalid_base64",
      path,
      message:
        "Expected strict canonical padded base64 without whitespace or a data-URL prefix.",
    });
    return;
  }

  const byteLength = decodedByteLength(base64);
  if (byteLength === 0) {
    issues.push({
      category: "semantic",
      code: "empty_image_payload",
      path,
      message: "Expected an embedded image with a non-empty decoded payload.",
    });
    return;
  }

  if (byteLength > MAX_EMBEDDED_IMAGE_BYTES) {
    issues.push({
      category: "semantic",
      code: "image_too_large",
      path,
      message: `Expected each decoded image to contain at most ${MAX_EMBEDDED_IMAGE_BYTES} bytes.`,
    });
  }

  budget.total += byteLength;
  if (budget.total > MAX_LAYOUT_IMAGE_BYTES && !budget.aggregateLimitReported) {
    budget.aggregateLimitReported = true;
    issues.push({
      category: "semantic",
      code: "aggregate_image_bytes_exceeded",
      path,
      message: `This image occurrence raises the combined decoded image size above ${MAX_LAYOUT_IMAGE_BYTES} bytes.`,
    });
  }

  if (signatureMatches(mediaType, decodePrefix(base64, 12))) return;
  issues.push({
    category: "semantic",
    code: "image_media_type_mismatch",
    path,
    message: `Decoded image signature does not match declared media type ${mediaType}.`,
  });
}
