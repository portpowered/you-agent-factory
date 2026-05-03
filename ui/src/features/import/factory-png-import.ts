import { normalizeFactoryDefinition } from "../../api/factory-definition";
import type { CanonicalFactoryDefinition } from "../../api/factory-definition";

const PNG_SIGNATURE = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
const PNG_TEXT_CHUNK = "tEXt";
const PNG_INTERNATIONAL_TEXT_CHUNK = "iTXt";
const PORT_OS_FACTORY_PNG_ITXT_MIN_FIELDS = 5;
const PORT_OS_FACTORY_PNG_ITXT_UNCOMPRESSED_FLAG = 0;

export const PORT_OS_FACTORY_PNG_METADATA_KEYWORD = "portos.agent-factory";
export const PORT_OS_FACTORY_PNG_SCHEMA_VERSION = "portos.agent-factory.png.v1";

export interface FactoryPngMetadata extends CanonicalFactoryDefinition {
  schemaVersion: typeof PORT_OS_FACTORY_PNG_SCHEMA_VERSION;
}

export type ReadFactoryImportPngErrorCode =
  | "FILE_READ_FAILED"
  | "NOT_PNG_FILE"
  | "PNG_INVALID"
  | "PNG_METADATA_MISSING"
  | "PNG_METADATA_INVALID"
  | "UNSUPPORTED_SCHEMA_VERSION"
  | "FACTORY_PAYLOAD_INVALID"
  | "IMAGE_DECODE_FAILED"
  | "PREVIEW_UNAVAILABLE";

export interface ReadFactoryImportPngError {
  cause?: unknown;
  code: ReadFactoryImportPngErrorCode;
  details?: {
    schemaVersion?: string;
  };
  message: string;
}

export interface ReadFactoryImportPngOptions {
  createPreviewImageSrc?: (file: Blob) => string;
  file: Blob;
  revokePreviewImageSrc?: (previewImageSrc: string) => void;
  validatePreviewImage?: (file: Blob) => Promise<void>;
}

export interface FactoryPngImportValue {
  factory: CanonicalFactoryDefinition;
  previewImageSrc: string;
  revokePreviewImageSrc: () => void;
  schemaVersion: typeof PORT_OS_FACTORY_PNG_SCHEMA_VERSION;
}

export interface ReadFactoryImportPngFailure {
  error: ReadFactoryImportPngError;
  ok: false;
}

export interface ReadFactoryImportPngSuccess {
  ok: true;
  value: FactoryPngImportValue;
}

export type ReadFactoryImportPngResult =
  | ReadFactoryImportPngFailure
  | ReadFactoryImportPngSuccess;

type ImportStepResult<T> = ReadFactoryImportPngFailure | { ok: true; value: T };

export async function readFactoryImportPng({
  createPreviewImageSrc = createPreviewImageSrcInBrowser,
  file,
  revokePreviewImageSrc = revokePreviewImageSrcInBrowser,
  validatePreviewImage = validatePreviewImageInBrowser,
}: ReadFactoryImportPngOptions): Promise<ReadFactoryImportPngResult> {
  const pngBytes = await readPngBytes(file);
  if (!pngBytes.ok) {
    return pngBytes;
  }

  const metadataText = readFactoryMetadataText(pngBytes.value);
  if (!metadataText.ok) {
    return metadataText;
  }

  const metadata = parseFactoryMetadata(metadataText.value);
  if (!metadata.ok) {
    return metadata;
  }

  try {
    await validatePreviewImage(file);
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for preview.",
      },
      ok: false,
    };
  }

  let previewImageSrc: string;
  try {
    previewImageSrc = createPreviewImageSrc(file);
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "PREVIEW_UNAVAILABLE",
        message: "The browser could not create a preview for the selected image.",
      },
      ok: false,
    };
  }

  return {
    ok: true,
    value: {
      factory: stripMetadataSchemaVersion(metadata.value),
      previewImageSrc,
      revokePreviewImageSrc: () => {
        revokePreviewImageSrc(previewImageSrc);
      },
      schemaVersion: metadata.value.schemaVersion,
    },
  };
}

async function readPngBytes(file: Blob): Promise<ImportStepResult<Uint8Array>> {
  let pngBytes: Uint8Array;
  try {
    pngBytes = await readBlobToUint8Array(file);
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "FILE_READ_FAILED",
        message: "The selected image could not be read.",
      },
      ok: false,
    };
  }

  if (!hasPngSignature(pngBytes)) {
    return {
      error: {
        code: "NOT_PNG_FILE",
        message: "The selected file is not a PNG image.",
      },
      ok: false,
    };
  }

  return {
    ok: true,
    value: pngBytes,
  };
}

function readFactoryMetadataText(pngBytes: Uint8Array): ImportStepResult<string> {
  try {
    let offset = PNG_SIGNATURE.length;

    while (offset < pngBytes.length) {
      const chunk = readChunkAtOffset(pngBytes, offset);
      offset = chunk.nextOffset;

      if (chunk.type !== PNG_TEXT_CHUNK && chunk.type !== PNG_INTERNATIONAL_TEXT_CHUNK) {
        continue;
      }

      const textChunk = readTextChunk(chunk.type, chunk.data);
      if (textChunk.keyword !== PORT_OS_FACTORY_PNG_METADATA_KEYWORD) {
        continue;
      }

      return {
        ok: true,
        value: textChunk.text,
      };
    }
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "PNG_INVALID",
        message: "The selected PNG image is invalid or truncated.",
      },
      ok: false,
    };
  }

  return {
    error: {
      code: "PNG_METADATA_MISSING",
      message: "The selected PNG does not contain Port OS factory metadata.",
    },
    ok: false,
  };
}

function parseFactoryMetadata(metadataText: string): ImportStepResult<FactoryPngMetadata> {
  let parsedMetadata: unknown;
  try {
    parsedMetadata = JSON.parse(metadataText);
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata is not valid JSON.",
      },
      ok: false,
    };
  }

  if (!isRecord(parsedMetadata)) {
    return {
      error: {
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata must be an object.",
      },
      ok: false,
    };
  }

  if (!isNonEmptyString(parsedMetadata.schemaVersion)) {
    return {
      error: {
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata is missing the schema version.",
      },
      ok: false,
    };
  }

  if (parsedMetadata.schemaVersion !== PORT_OS_FACTORY_PNG_SCHEMA_VERSION) {
    return {
      error: {
        code: "UNSUPPORTED_SCHEMA_VERSION",
        details: {
          schemaVersion: parsedMetadata.schemaVersion,
        },
        message: "The selected PNG uses an unsupported Port OS factory metadata version.",
      },
      ok: false,
    };
  }

  const normalizedMetadata = normalizeFactoryMetadata(parsedMetadata);
  if (!normalizedMetadata.ok) {
    return normalizedMetadata;
  }

  return {
    ok: true,
    value: normalizedMetadata.value,
  };
}

function normalizeFactoryMetadata(
  parsedMetadata: Record<string, unknown>,
): ImportStepResult<FactoryPngMetadata> {
  let normalizedFactory: CanonicalFactoryDefinition;
  try {
    normalizedFactory = normalizeFactoryPayload(parsedMetadata);
  } catch {
    return {
      error: {
        code: "FACTORY_PAYLOAD_INVALID",
        message: "The Port OS factory metadata does not contain a valid factory payload.",
      },
      ok: false,
    };
  }

  const normalizedFactoryName = readFactoryMetadataName(parsedMetadata);
  if (!normalizedFactoryName.ok) {
    return normalizedFactoryName;
  }

  return {
    ok: true,
    value: {
      ...normalizedFactory,
      schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
    },
  };
}

function normalizeFactoryPayload(
  parsedMetadata: Record<string, unknown>,
): CanonicalFactoryDefinition {
  const { schemaVersion: _schemaVersion, ...factoryPayload } = parsedMetadata;
  return normalizeFactoryDefinition(factoryPayload);
}

function readFactoryMetadataName(parsedMetadata: Record<string, unknown>): ImportStepResult<string> {
  const canonicalFactoryName = readCanonicalFactoryMetadataName(parsedMetadata);
  if (canonicalFactoryName !== null) {
    return {
      ok: true,
      value: canonicalFactoryName,
    };
  }

  return {
    error: {
      code: "PNG_METADATA_INVALID",
      message: "The Port OS factory metadata is missing the factory name.",
    },
    ok: false,
  };
}

async function validatePreviewImageInBrowser(file: Blob): Promise<void> {
  const bitmapFactory = globalThis.createImageBitmap;
  if (typeof bitmapFactory === "function") {
    const bitmap = await bitmapFactory(file);
    bitmap.close();
    return;
  }

  if (typeof document !== "undefined" && typeof URL.createObjectURL === "function") {
    const objectUrl = URL.createObjectURL(file);
    try {
      await loadImageElement(objectUrl);
      return;
    } finally {
      URL.revokeObjectURL(objectUrl);
    }
  }

  throw new Error("Browser image decoding is unavailable.");
}

async function readBlobToUint8Array(file: Blob): Promise<Uint8Array> {
  if (typeof file.arrayBuffer === "function") {
    return new Uint8Array(await file.arrayBuffer());
  }

  if (typeof FileReader === "undefined") {
    throw new Error("Blob arrayBuffer and FileReader are unavailable.");
  }

  return await new Promise<Uint8Array>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (reader.result instanceof ArrayBuffer) {
        resolve(new Uint8Array(reader.result));
        return;
      }

      reject(new Error("FileReader did not return an ArrayBuffer."));
    };
    reader.onerror = () => {
      reject(reader.error ?? new Error("FileReader failed."));
    };
    reader.readAsArrayBuffer(file);
  });
}

function createPreviewImageSrcInBrowser(file: Blob): string {
  if (typeof URL.createObjectURL !== "function") {
    throw new Error("Browser preview URL creation is unavailable.");
  }

  return URL.createObjectURL(file);
}

function revokePreviewImageSrcInBrowser(previewImageSrc: string): void {
  if (typeof URL.revokeObjectURL === "function") {
    URL.revokeObjectURL(previewImageSrc);
  }
}

async function loadImageElement(sourceUrl: string): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      resolve();
    };
    image.onerror = () => {
      reject(new Error("Image load failed."));
    };
    image.src = sourceUrl;
  });
}

function readChunkAtOffset(pngBytes: Uint8Array, offset: number): ParsedChunk {
  if (offset + 8 > pngBytes.length) {
    throw new Error("PNG chunk header is truncated.");
  }

  const view = new DataView(pngBytes.buffer, pngBytes.byteOffset, pngBytes.byteLength);
  const length = view.getUint32(offset);
  const dataOffset = offset + 8;
  const dataEnd = dataOffset + length;
  const chunkEnd = dataEnd + 4;

  if (chunkEnd > pngBytes.length) {
    throw new Error("PNG chunk payload is truncated.");
  }

  return {
    data: pngBytes.slice(dataOffset, dataEnd),
    nextOffset: chunkEnd,
    type: decodeAscii(pngBytes.subarray(offset + 4, offset + 8)),
  };
}

function readTextChunk(type: string, data: Uint8Array): { keyword: string; text: string } {
  if (type === PNG_TEXT_CHUNK) {
    return readTextKeywordAndValue(data);
  }

  return readInternationalTextKeywordAndValue(data);
}

function readTextKeywordAndValue(data: Uint8Array): { keyword: string; text: string } {
  const keywordEnd = data.indexOf(0);
  if (keywordEnd < 0) {
    throw new Error("PNG tEXt chunk keyword is invalid.");
  }

  return {
    keyword: decodeAscii(data.subarray(0, keywordEnd)),
    text: new TextDecoder().decode(data.subarray(keywordEnd + 1)),
  };
}

function readInternationalTextKeywordAndValue(data: Uint8Array): { keyword: string; text: string } {
  const keywordEnd = data.indexOf(0);
  if (keywordEnd < 0) {
    throw new Error("PNG iTXt chunk keyword is invalid.");
  }

  if (data.length < keywordEnd + PORT_OS_FACTORY_PNG_ITXT_MIN_FIELDS + 1) {
    throw new Error("PNG iTXt chunk is truncated.");
  }

  const compressionFlagIndex = keywordEnd + 1;
  const compressionFlag = data[compressionFlagIndex];
  if (compressionFlag !== PORT_OS_FACTORY_PNG_ITXT_UNCOMPRESSED_FLAG) {
    throw new Error("Compressed PNG iTXt metadata is not supported.");
  }

  const languageTagEnd = data.indexOf(0, keywordEnd + 3);
  if (languageTagEnd < 0) {
    throw new Error("PNG iTXt chunk language tag is invalid.");
  }

  const translatedKeywordEnd = data.indexOf(0, languageTagEnd + 1);
  if (translatedKeywordEnd < 0) {
    throw new Error("PNG iTXt chunk translated keyword is invalid.");
  }

  return {
    keyword: decodeAscii(data.subarray(0, keywordEnd)),
    text: new TextDecoder().decode(data.subarray(translatedKeywordEnd + 1)),
  };
}

function hasPngSignature(pngBytes: Uint8Array): boolean {
  if (pngBytes.byteLength < PNG_SIGNATURE.byteLength) {
    return false;
  }

  return PNG_SIGNATURE.every((value, index) => pngBytes[index] === value);
}

function decodeAscii(value: Uint8Array): string {
  return String.fromCharCode(...value);
}

function readCanonicalFactoryMetadataName(value: Record<string, unknown>): string | null {
  if (!isNonEmptyString(value.name)) {
    return null;
  }

  return value.name.trim();
}

function stripMetadataSchemaVersion(metadata: FactoryPngMetadata): CanonicalFactoryDefinition {
  const { schemaVersion: _schemaVersion, ...factory } = metadata;
  return factory;
}

function _isStringMap(value: unknown): value is Record<string, string> | undefined {
  if (value === undefined) {
    return true;
  }

  if (!isRecord(value)) {
    return false;
  }

  return Object.values(value).every((entry) => typeof entry === "string");
}

function _isOptionalArray<T>(value: unknown, predicate: (entry: unknown) => boolean): value is T[] | undefined {
  return value === undefined || (Array.isArray(value) && value.every((entry) => predicate(entry)));
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

interface ParsedChunk {
  data: Uint8Array;
  nextOffset: number;
  type: string;
}
