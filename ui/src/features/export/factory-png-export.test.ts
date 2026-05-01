import type { components } from "../../api/generated/openapi";
import {
  PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
  PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
  writeFactoryExportPng,
} from "./factory-png-export";

type FactorySchemas = components["schemas"];

const ONE_PIXEL_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==";

const canonicalFactory: FactorySchemas["Factory"] = {
  id: "agent-factory",
  name: "agent-factory",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [
    {
      executorProvider: "script_wrap",
      model: "codex-mini",
      modelProvider: "codex",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      inputs: [{ state: "new", workType: "story" }],
      name: "draft",
      onFailure: { state: "done", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "writer",
    },
  ],
};

describe("writeFactoryExportPng", () => {
  it("writes the Port OS metadata envelope without changing the source PNG image data", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const result = await writeFactoryExportPng({
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
      namedFactory: {
        factory: canonicalFactory,
        name: "Factory Export",
      },
      rasterizeImageToPngBytes: async () => sourcePng,
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      throw new Error("expected successful PNG export");
    }

    const exportedBytes = await blobToUint8Array(result.blob);
    const exportedChunks = parsePngChunks(exportedBytes);
    const metadataChunk = exportedChunks.find((chunk) => chunk.type === "iTXt");
    const sourceIDAT = parsePngChunks(sourcePng).find((chunk) => chunk.type === "IDAT");
    const exportedIDAT = exportedChunks.find((chunk) => chunk.type === "IDAT");

    expect(metadataChunk).toBeDefined();
    expect(readInternationalTextChunk(metadataChunk?.data ?? new Uint8Array())).toEqual({
      keyword: PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
      text: JSON.stringify({
        factory: canonicalFactory,
        name: "Factory Export",
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      }),
    });
    expect(exportedIDAT?.data).toEqual(sourceIDAT?.data);
  });

  it("returns an explicit decode failure when the image cannot be rasterized", async () => {
    const result = await writeFactoryExportPng({
      image: new Blob(["not-an-image"], { type: "text/plain" }),
      namedFactory: {
        factory: canonicalFactory,
        name: "Broken Export",
      },
      rasterizeImageToPngBytes: async () => {
        throw new Error("decode failed");
      },
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(Error),
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for PNG export.",
      },
      ok: false,
    });
  });

  it("returns an explicit metadata-write failure when the rasterized bytes are not a PNG", async () => {
    const result = await writeFactoryExportPng({
      image: new Blob(["not-a-png"], { type: "image/png" }),
      namedFactory: {
        factory: canonicalFactory,
        name: "Broken Export",
      },
      rasterizeImageToPngBytes: async () => new Uint8Array([1, 2, 3, 4]),
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(Error),
        code: "PNG_METADATA_WRITE_FAILED",
        message: "The exported PNG metadata could not be written.",
      },
      ok: false,
    });
  });
});

function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

async function blobToUint8Array(blob: Blob): Promise<Uint8Array> {
  return await new Promise<Uint8Array>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (reader.result instanceof ArrayBuffer) {
        resolve(new Uint8Array(reader.result));
        return;
      }
      reject(new Error("Blob reader did not return an ArrayBuffer."));
    };
    reader.onerror = () => {
      reject(reader.error ?? new Error("Blob reader failed."));
    };
    reader.readAsArrayBuffer(blob);
  });
}

function parsePngChunks(pngBytes: Uint8Array): ParsedChunk[] {
  const chunks: ParsedChunk[] = [];
  let offset = 8;
  const view = new DataView(pngBytes.buffer, pngBytes.byteOffset, pngBytes.byteLength);

  while (offset < pngBytes.byteLength) {
    const length = view.getUint32(offset);
    const type = String.fromCharCode(...pngBytes.subarray(offset + 4, offset + 8));
    const dataStart = offset + 8;
    const dataEnd = dataStart + length;

    chunks.push({
      data: pngBytes.slice(dataStart, dataEnd),
      type,
    });
    offset = dataEnd + 4;
  }

  return chunks;
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}

function readInternationalTextChunk(data: Uint8Array): { keyword: string; text: string } {
  const keywordEnd = data.indexOf(0);
  const keyword = String.fromCharCode(...data.subarray(0, keywordEnd));
  const textStart = keywordEnd + 5;

  return {
    keyword,
    text: new TextDecoder().decode(data.subarray(textStart)),
  };
}

interface ParsedChunk {
  data: Uint8Array;
  type: string;
}
