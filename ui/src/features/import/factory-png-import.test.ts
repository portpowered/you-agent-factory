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
      executorProvider: "SCRIPT_WRAP",
      model: "codex-mini",
      modelProvider: "CODEX",
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

describe("readFactoryImportPng", () => {
  it("returns one normalized import result for a valid Port OS factory PNG", async () => {
    const previewUrl = "blob:factory-preview";
    const revokePreviewImageSrc = vi.fn();
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => previewUrl,
      file: createFactoryPngFile({
        factory: {
          ...canonicalFactory,
          name: "Factory Import",
        },
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      }),
      revokePreviewImageSrc,
      validatePreviewImage: async () => {},
    });

    expect(result.ok).toBe(true);
    if (!result.ok) {
      throw new Error("expected successful PNG import");
    }

    expect(result.value.factory).toEqual({
      ...canonicalFactory,
      name: "Factory Import",
    });
    expect(result.value.previewImageSrc).toBe(previewUrl);
    expect(result.value.schemaVersion).toBe(PORT_OS_FACTORY_PNG_SCHEMA_VERSION);

    result.value.revokePreviewImageSrc();
    expect(revokePreviewImageSrc).toHaveBeenCalledWith(previewUrl);
  });

  it("rejects the retired factoryName envelope fallback", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => "blob:legacy-preview",
      file: createFactoryPngFileWithMetadataText(
        JSON.stringify({
          ...canonicalFactory,
          factoryName: "Legacy Factory Import",
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      ),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "FACTORY_PAYLOAD_INVALID",
        message: "The Port OS factory metadata does not contain a valid factory payload.",
      },
      ok: false,
    });
  });

  it("rejects factory payloads that fall outside the generated contract", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: createFactoryPngFileWithMetadataText(
        JSON.stringify({
          exhaustion_rules: [],
          project: "legacy-factory",
          name: "Invalid Factory Import",
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      ),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "FACTORY_PAYLOAD_INVALID",
        message: "The Port OS factory metadata does not contain a valid factory payload.",
      },
      ok: false,
    });
  });

  it("rejects PNG metadata that falls back to the legacy top-level factory payload", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: createFactoryPngFileWithMetadataText(
        JSON.stringify({
          id: "legacy-factory",
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
          workTypes: canonicalFactory.workTypes,
          workers: canonicalFactory.workers,
          workstations: canonicalFactory.workstations,
        }),
      ),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "FACTORY_PAYLOAD_INVALID",
        message: "The Port OS factory metadata does not contain a valid factory payload.",
      },
      ok: false,
    });
  });

  it("rejects non-PNG files before preview creation", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: new File(["not-a-png"], "factory.txt", { type: "text/plain" }),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "NOT_PNG_FILE",
        message: "The selected file is not a PNG image.",
      },
      ok: false,
    });
  });

  it("rejects PNG files that are missing the Port OS metadata chunk", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: new File([toArrayBuffer(fromBase64(ONE_PIXEL_PNG_BASE64))], "plain.png", {
        type: "image/png",
      }),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "PNG_METADATA_MISSING",
        message: "The selected PNG does not contain Port OS factory metadata.",
      },
      ok: false,
    });
  });

  it("rejects unsupported metadata schema versions with structured details", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: createFactoryPngFile({
        factory: {
          ...canonicalFactory,
          name: "Factory Import",
        },
        schemaVersion: "portos.agent-factory.png.v2",
      }),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        code: "UNSUPPORTED_SCHEMA_VERSION",
        details: {
          schemaVersion: "portos.agent-factory.png.v2",
        },
        message: "The selected PNG uses an unsupported Port OS factory metadata version.",
      },
      ok: false,
    });
  });

  it("returns an explicit decode failure when preview validation cannot decode the image", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: createFactoryPngFile({
        factory: {
          ...canonicalFactory,
          name: "Factory Import",
        },
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      }),
      validatePreviewImage: async () => {
        throw new Error("decode failed");
      },
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

  it("rejects malformed metadata JSON with an explicit local error code", async () => {
    const result = await readFactoryImportPng({
      createPreviewImageSrc: () => {
        throw new Error("should not create preview");
      },
      file: createFactoryPngFileWithMetadataText("{not valid json"),
      validatePreviewImage: async () => {},
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(SyntaxError),
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata is not valid JSON.",
      },
      ok: false,
    });
  });
});

function createFactoryPngFile({
  factory,
  schemaVersion,
}: {
  factory: FactorySchemas["Factory"];
  schemaVersion: string;
}): File {
  return createFactoryPngFileWithMetadataText(
    JSON.stringify({
      ...factory,
      schemaVersion,
    }),
  );
}

function createFactoryPngFileWithMetadataText(metadataText: string): File {
  const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
  const pngWithMetadata = injectMetadataChunk(
    sourcePng,
    buildChunk("iTXt", buildInternationalTextData(PORT_OS_FACTORY_PNG_METADATA_KEYWORD, metadataText)),
  );

  return new File([toArrayBuffer(pngWithMetadata)], "factory.png", { type: "image/png" });
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

