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
      onContinue: [
        { state: "new", workType: "story" },
        { state: "queued", workType: "story" },
      ],
      onFailure: [
        { state: "done", workType: "story" },
        { state: "blocked", workType: "story" },
      ],
      onRejection: [
        { state: "retry", workType: "story" },
        { state: "backlog", workType: "story" },
      ],
      outputs: [{ state: "done", workType: "story" }],
      worker: "writer",
    },
  ],
};

describe("writeFactoryExportPng", () => {
  it("writes the Port OS factory metadata without changing the source PNG image data", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const result = await writeFactoryExportPng({
      factory: {
        ...canonicalFactory,
        name: "Factory Export",
      },
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
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
        ...canonicalFactory,
        name: "Factory Export",
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      }),
    });
    expect(exportedIDAT?.data).toEqual(sourceIDAT?.data);
  });

  it("returns an explicit decode failure when the image cannot be rasterized", async () => {
    const result = await writeFactoryExportPng({
      factory: {
        ...canonicalFactory,
        name: "Broken Export",
      },
      image: new Blob(["not-an-image"], { type: "text/plain" }),
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
      factory: {
        ...canonicalFactory,
        name: "Broken Export",
      },
      image: new Blob(["not-a-png"], { type: "image/png" }),
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

  it("uses createImageBitmap and OffscreenCanvas when the browser exposes them", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const close = vi.fn();
    const drawImage = vi.fn();
    const convertToBlob = vi.fn(async () => ({
      arrayBuffer: async () => toArrayBuffer(sourcePng),
    }));

    vi.stubGlobal(
      "createImageBitmap",
      vi.fn(async () => ({
        close,
        height: 1,
        width: 1,
      })),
    );
    vi.stubGlobal(
      "OffscreenCanvas",
      class MockOffscreenCanvas {
        public constructor(
          public readonly width: number,
          public readonly height: number,
        ) {}

        public getContext(_contextID: "2d"): OffscreenCanvasRenderingContext2D {
          return {
            drawImage,
          } as OffscreenCanvasRenderingContext2D;
        }

        public async convertToBlob(): Promise<Blob> {
          return await convertToBlob();
        }
      },
    );

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
    });

    if (!result.ok) {
      throw result.error.cause instanceof Error ? result.error.cause : new Error(result.error.message);
    }
    expect(result.ok).toBe(true);
    expect(drawImage).toHaveBeenCalledTimes(1);
    expect(convertToBlob).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
  });

  it("falls back to HTML image and canvas rendering when createImageBitmap is unavailable", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const createObjectURL = vi.fn(() => "blob:factory-export");
    const revokeObjectURL = vi.fn();
    const drawImage = vi.fn();
    const originalCreateElement = document.createElement.bind(document);
    const getContext = vi.fn(() => ({
      drawImage,
    }));
    const toBlob = vi.fn((callback: BlobCallback) => {
      callback({
        arrayBuffer: async () => toArrayBuffer(sourcePng),
      } as Blob);
    });

    vi.stubGlobal("createImageBitmap", undefined);
    vi.stubGlobal("OffscreenCanvas", undefined);
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL,
      revokeObjectURL,
    });
    vi.stubGlobal(
      "Image",
      class MockImage {
        public naturalHeight = 1;
        public naturalWidth = 1;
        public onerror: (() => void) | null = null;
        public onload: (() => void) | null = null;

        public set src(_value: string) {
          this.onload?.();
        }
      },
    );
    vi.spyOn(document, "createElement").mockImplementation(((tagName: string) => {
      if (tagName === "canvas") {
        return {
          getContext,
          height: 0,
          toBlob,
          width: 0,
        } as unknown as HTMLCanvasElement;
      }

      return originalCreateElement(tagName);
    }) as typeof document.createElement);

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
    });

    if (!result.ok) {
      throw result.error.cause instanceof Error ? result.error.cause : new Error(result.error.message);
    }
    expect(result.ok).toBe(true);
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(getContext).toHaveBeenCalledWith("2d");
    expect(drawImage).toHaveBeenCalledTimes(1);
    expect(toBlob).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:factory-export");
  });

  it("returns a decode failure when the browser fallback image loader fails", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const revokeObjectURL = vi.fn();

    vi.stubGlobal("createImageBitmap", undefined);
    vi.stubGlobal("OffscreenCanvas", undefined);
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => "blob:factory-export"),
      revokeObjectURL,
    });
    vi.stubGlobal(
      "Image",
      class MockImage {
        public onerror: (() => void) | null = null;
        public onload: (() => void) | null = null;

        public set src(_value: string) {
          this.onerror?.();
        }
      },
    );

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(Error),
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for PNG export.",
      },
      ok: false,
    });
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:factory-export");
  });

  it("returns a decode failure when the browser canvas context is unavailable", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);

    vi.stubGlobal(
      "createImageBitmap",
      vi.fn(async () => ({
        close: vi.fn(),
        height: 1,
        width: 1,
      })),
    );
    vi.stubGlobal(
      "OffscreenCanvas",
      class MockOffscreenCanvas {
        public constructor(
          public readonly width: number,
          public readonly height: number,
        ) {}

        public getContext(_contextID: "2d"): null {
          return null;
        }
      },
    );

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
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

  it("returns a decode failure when decoded image dimensions are invalid", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);

    vi.stubGlobal(
      "createImageBitmap",
      vi.fn(async () => ({
        close: vi.fn(),
        height: 0,
        width: 1,
      })),
    );

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
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

  it("returns a decode failure when HTML canvas encoding does not produce a blob", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const originalCreateElement = document.createElement.bind(document);

    vi.stubGlobal("createImageBitmap", undefined);
    vi.stubGlobal("OffscreenCanvas", undefined);
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: vi.fn(() => "blob:factory-export"),
      revokeObjectURL: vi.fn(),
    });
    vi.stubGlobal(
      "Image",
      class MockImage {
        public naturalHeight = 1;
        public naturalWidth = 1;
        public onerror: (() => void) | null = null;
        public onload: (() => void) | null = null;

        public set src(_value: string) {
          this.onload?.();
        }
      },
    );
    vi.spyOn(document, "createElement").mockImplementation(((tagName: string) => {
      if (tagName === "canvas") {
        return {
          getContext: () => ({
            drawImage() {},
          }),
          height: 0,
          toBlob: (callback: BlobCallback) => {
            callback(null);
          },
          width: 0,
        } as unknown as HTMLCanvasElement;
      }

      return originalCreateElement(tagName);
    }) as typeof document.createElement);

    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
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

  it("replaces existing Infinite You metadata instead of appending duplicate metadata chunks", async () => {
    const sourcePng = fromBase64(ONE_PIXEL_PNG_BASE64);
    const firstExport = await writeFactoryExportPng({
      factory: {
        ...canonicalFactory,
        name: "Factory Export",
      },
      image: new Blob([toArrayBuffer(sourcePng)], { type: "image/png" }),
      rasterizeImageToPngBytes: async () => sourcePng,
    });

    expect(firstExport.ok).toBe(true);
    if (!firstExport.ok) {
      throw new Error("expected initial export to succeed");
    }

    const secondExport = await writeFactoryExportPng({
      factory: {
        ...canonicalFactory,
        name: "Factory Export Updated",
      },
      image: firstExport.blob,
      rasterizeImageToPngBytes: async () => await blobToUint8Array(firstExport.blob),
    });

    expect(secondExport.ok).toBe(true);
    if (!secondExport.ok) {
      throw new Error("expected replacement export to succeed");
    }

    const metadataChunks = parsePngChunks(await blobToUint8Array(secondExport.blob)).filter(
      (chunk) => chunk.type === "iTXt",
    );

    expect(metadataChunks).toHaveLength(1);
    expect(readInternationalTextChunk(metadataChunks[0]?.data ?? new Uint8Array())).toEqual({
      keyword: PORT_OS_FACTORY_PNG_METADATA_KEYWORD,
      text: JSON.stringify({
        ...canonicalFactory,
        name: "Factory Export Updated",
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      }),
    });
  });
});

function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

async function blobToUint8Array(blob: Blob): Promise<Uint8Array> {
  if (typeof blob.arrayBuffer === "function") {
    return new Uint8Array(await blob.arrayBuffer());
  }

  if (typeof FileReader === "undefined") {
    throw new Error("Blob readers are unavailable.");
  }

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
