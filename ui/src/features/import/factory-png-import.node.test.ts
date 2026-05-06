// @vitest-environment node

import type { components } from "../../api/generated/openapi";
import {
  PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
  PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
  readFactoryImportPng,
} from "./factory-png-import";

type FactorySchemas = components["schemas"];

const ONE_PIXEL_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==";
const PNG_SIGNATURE = new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]);

const canonicalFactory: FactorySchemas["Factory"] = {
  id: "agent-factory",
  name: "agent-factory",
  workTypes: [],
  workers: [],
  workstations: [],
};

describe("readFactoryImportPng outside browser preview contexts", () => {
  it("returns an explicit decode failure when no browser preview decoder is available", async () => {
    const result = await readFactoryImportPng({
      file: createFactoryPngFile(),
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(Error),
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for preview.",
      },
      ok: false,
    });
  });
});

function createFactoryPngFile(): File {
  return new File(
    [
      toArrayBuffer(
        injectMetadataChunk(
          fromBase64(ONE_PIXEL_PNG_BASE64),
          buildChunk(
            "iTXt",
            buildInternationalTextData(
              PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
              JSON.stringify({
                ...canonicalFactory,
                schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
              }),
            ),
          ),
        ),
      ),
    ],
    "factory.png",
    { type: "image/png" },
  );
}

function buildInternationalTextData(keyword: string, text: string): Uint8Array {
  const encoder = new TextEncoder();
  return concatBytes([
    encoder.encode(keyword),
    new Uint8Array([0, 0, 0, 0, 0]),
    encoder.encode(text),
  ]);
}

function injectMetadataChunk(pngBytes: Uint8Array, metadataChunk: Uint8Array): Uint8Array {
  const chunks: Uint8Array[] = [PNG_SIGNATURE];
  let offset = PNG_SIGNATURE.length;
  let inserted = false;

  while (offset < pngBytes.length) {
    const chunk = readChunkAtOffset(pngBytes, offset);
    offset = chunk.nextOffset;
    chunks.push(chunk.bytes);

    if (!inserted && chunk.type === "IHDR") {
      chunks.push(metadataChunk);
      inserted = true;
    }
  }

  return concatBytes(chunks);
}

function readChunkAtOffset(pngBytes: Uint8Array, offset: number): ParsedChunk {
  const view = new DataView(pngBytes.buffer, pngBytes.byteOffset, pngBytes.byteLength);
  const length = view.getUint32(offset);
  const dataOffset = offset + 8;
  const chunkEnd = dataOffset + length + 4;

  return {
    bytes: pngBytes.slice(offset, chunkEnd),
    nextOffset: chunkEnd,
    type: String.fromCharCode(...pngBytes.subarray(offset + 4, offset + 8)),
  };
}

function buildChunk(type: string, data: Uint8Array): Uint8Array {
  const typeBytes = new TextEncoder().encode(type);
  const lengthBytes = new Uint8Array(4);
  new DataView(lengthBytes.buffer).setUint32(0, data.byteLength);
  const crcBytes = new Uint8Array(4);

  return concatBytes([lengthBytes, typeBytes, data, crcBytes]);
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

function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}

interface ParsedChunk {
  bytes: Uint8Array;
  nextOffset: number;
  type: string;
}
