import type { NamedFactoryValue } from "../../api/named-factory";

const PNG_SIGNATURE = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);
const PNG_TEXT_CHUNK = "tEXt";
const PNG_INTERNATIONAL_TEXT_CHUNK = "iTXt";

export const PORT_OS_FACTORY_PNG_METADATA_KEYWORD = "portos.agent-factory";
export const PORT_OS_FACTORY_PNG_SCHEMA_VERSION = "portos.agent-factory.png.v1";

export type CanonicalFactoryDefinition = NamedFactoryValue;
export interface PortOSFactoryPngEnvelope extends NamedFactoryValue {
  schemaVersion: typeof PORT_OS_FACTORY_PNG_SCHEMA_VERSION;
}

export interface WriteFactoryExportPngOptions {
  image: Blob;
  namedFactory: NamedFactoryValue;
  rasterizeImageToPngBytes?: (image: Blob) => Promise<Uint8Array>;
}

export interface WriteFactoryExportPngSuccess {
  blob: Blob;
  envelope: PortOSFactoryPngEnvelope;
  ok: true;
}

export interface WriteFactoryExportPngFailure {
  error: FactoryExportPngError;
  ok: false;
}

export type WriteFactoryExportPngResult =
  | WriteFactoryExportPngFailure
  | WriteFactoryExportPngSuccess;

export interface FactoryExportPngError {
  cause?: unknown;
  code: "IMAGE_DECODE_FAILED" | "PNG_METADATA_WRITE_FAILED";
  message: string;
}

export async function writeFactoryExportPng({
  image,
  namedFactory,
  rasterizeImageToPngBytes = rasterizeImageToPngBytesInBrowser,
}: WriteFactoryExportPngOptions): Promise<WriteFactoryExportPngResult> {
  const envelope: PortOSFactoryPngEnvelope = {
    ...namedFactory,
    schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
  };

  let pngBytes: Uint8Array;
  try {
    pngBytes = await rasterizeImageToPngBytes(image);
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for PNG export.",
      },
      ok: false,
    };
  }

  try {
    const metadataChunk = buildInternationalTextChunk(
      PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
      JSON.stringify(envelope),
    );
    const pngWithMetadata = injectMetadataChunk(pngBytes, metadataChunk);

    return {
      blob: new Blob([toArrayBuffer(pngWithMetadata)], { type: "image/png" }),
      envelope,
      ok: true,
    };
  } catch (error) {
    return {
      error: {
        cause: error,
        code: "PNG_METADATA_WRITE_FAILED",
        message: "The exported PNG metadata could not be written.",
      },
      ok: false,
    };
  }
}

async function rasterizeImageToPngBytesInBrowser(image: Blob): Promise<Uint8Array> {
  const bitmapFactory = globalThis.createImageBitmap;
  if (typeof bitmapFactory === "function") {
    const bitmap = await bitmapFactory(image);

    try {
      return await drawRasterToPngBytes(bitmap, bitmap.width, bitmap.height);
    } finally {
      bitmap.close();
    }
  }

  if (typeof document !== "undefined") {
    const objectUrl = URL.createObjectURL(image);

    try {
      const loadedImage = await loadImageElement(objectUrl);
      return await drawRasterToPngBytes(
        loadedImage,
        loadedImage.naturalWidth,
        loadedImage.naturalHeight,
      );
    } finally {
      URL.revokeObjectURL(objectUrl);
    }
  }

  throw new Error("Browser image decoding is unavailable.");
}

async function loadImageElement(sourceUrl: string): Promise<HTMLImageElement> {
  return await new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      resolve(image);
    };
    image.onerror = () => {
      reject(new Error("Image load failed."));
    };
    image.src = sourceUrl;
  });
}

async function drawRasterToPngBytes(
  source: CanvasImageSource,
  width: number,
  height: number,
): Promise<Uint8Array> {
  if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) {
    throw new Error("Decoded image dimensions are invalid.");
  }

  if (typeof OffscreenCanvas !== "undefined") {
    const canvas = new OffscreenCanvas(width, height);
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("PNG export canvas context is unavailable.");
    }
    context.drawImage(source, 0, 0, width, height);
    const blob = await canvas.convertToBlob({ type: "image/png" });
    return new Uint8Array(await blob.arrayBuffer());
  }

  if (typeof document === "undefined") {
    throw new Error("Canvas rendering is unavailable.");
  }

  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;

  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("PNG export canvas context is unavailable.");
  }

  context.drawImage(source, 0, 0, width, height);

  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((pngBlob) => {
      if (pngBlob) {
        resolve(pngBlob);
        return;
      }
      reject(new Error("Canvas PNG encoding failed."));
    }, "image/png");
  });

  return new Uint8Array(await blob.arrayBuffer());
}

function injectMetadataChunk(pngBytes: Uint8Array, metadataChunk: Uint8Array): Uint8Array {
  if (!hasPngSignature(pngBytes)) {
    throw new Error("Decoded image is not a PNG.");
  }

  const chunks: Uint8Array[] = [PNG_SIGNATURE];
  let inserted = false;
  let sawIHDR = false;
  let offset = PNG_SIGNATURE.length;

  while (offset < pngBytes.length) {
    const chunk = readChunkAtOffset(pngBytes, offset);
    offset = chunk.nextOffset;

    if (chunk.type === "IHDR") {
      sawIHDR = true;
      chunks.push(chunk.bytes);
      if (!inserted) {
        chunks.push(metadataChunk);
        inserted = true;
      }
      continue;
    }

    if (
      (chunk.type === PNG_TEXT_CHUNK || chunk.type === PNG_INTERNATIONAL_TEXT_CHUNK) &&
      readChunkKeyword(chunk.type, chunk.data) === PORT_OS_FACTORY_PNG_METADATA_KEYWORD
    ) {
      continue;
    }

    chunks.push(chunk.bytes);
  }

  if (!sawIHDR || !inserted) {
    throw new Error("Decoded PNG is missing an IHDR chunk.");
  }

  return concatBytes(chunks);
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
    bytes: pngBytes.slice(offset, chunkEnd),
    data: pngBytes.slice(dataOffset, dataEnd),
    nextOffset: chunkEnd,
    type: decodeAscii(pngBytes.subarray(offset + 4, offset + 8)),
  };
}

function buildInternationalTextChunk(keyword: string, text: string): Uint8Array {
  const encoder = new TextEncoder();
  const keywordBytes = encoder.encode(keyword);
  const textBytes = encoder.encode(text);
  const data = concatBytes([
    keywordBytes,
    new Uint8Array([0, 0, 0, 0, 0]),
    textBytes,
  ]);

  return buildChunk(PNG_INTERNATIONAL_TEXT_CHUNK, data);
}

function buildChunk(type: string, data: Uint8Array): Uint8Array {
  const typeBytes = new TextEncoder().encode(type);
  const lengthBytes = new Uint8Array(4);
  new DataView(lengthBytes.buffer).setUint32(0, data.byteLength);

  const crcInput = concatBytes([typeBytes, data]);
  const crcBytes = new Uint8Array(4);
  new DataView(crcBytes.buffer).setUint32(0, crc32(crcInput));

  return concatBytes([lengthBytes, typeBytes, data, crcBytes]);
}

function readChunkKeyword(type: string, data: Uint8Array): string | null {
  if (type !== PNG_TEXT_CHUNK && type !== PNG_INTERNATIONAL_TEXT_CHUNK) {
    return null;
  }

  const nullIndex = data.indexOf(0);
  if (nullIndex < 0) {
    return null;
  }

  return decodeAscii(data.subarray(0, nullIndex));
}

function hasPngSignature(pngBytes: Uint8Array): boolean {
  if (pngBytes.byteLength < PNG_SIGNATURE.byteLength) {
    return false;
  }

  return PNG_SIGNATURE.every((value, index) => pngBytes[index] === value);
}

function concatBytes(chunks: Uint8Array[]): Uint8Array {
  const totalLength = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0);
  const combined = new Uint8Array(totalLength);
  let offset = 0;

  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.byteLength;
  }

  return combined;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}

function decodeAscii(value: Uint8Array): string {
  return String.fromCharCode(...value);
}

function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff;

  for (const value of bytes) {
    crc = CRC32_TABLE[(crc ^ value) & 0xff] ^ (crc >>> 8);
  }

  return (crc ^ 0xffffffff) >>> 0;
}

interface ParsedChunk {
  bytes: Uint8Array;
  data: Uint8Array;
  nextOffset: number;
  type: string;
}

const CRC32_TABLE = buildCrc32Table();

function buildCrc32Table(): Uint32Array {
  const table = new Uint32Array(256);

  for (let index = 0; index < table.length; index += 1) {
    let value = index;
    for (let bit = 0; bit < 8; bit += 1) {
      value = (value & 1) === 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
    }
    table[index] = value >>> 0;
  }

  return table;
}
