import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";
import type { CurrentFactoryDocument } from "../api/current-factory-definition";
import type { FactoryValue } from "../api/session-factory";
import { defaultSessionFactoryVersion } from "./session-factory-mocks";

const onePixelPngBase64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==";

export const exportTimelineEvents: FactoryEvent[] = [
  {
    context: {
      eventTime: "2026-04-16T12:00:00Z",
      sequence: 0,
      tick: 0,
    },
    id: "export-run-request",
    payload: {
      factory: {
        id: "semantic-workflow",
        name: "semantic-workflow",
        factoryDirectory: "/work/factories/semantic-workflow",
        workers: [
          {
            modelProvider: "CODEX",
            name: "reviewer",
            executorProvider: "SCRIPT_WRAP",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "new", workType: "story" }],
            name: "Review",
            onFailure: [{ state: "done", workType: "story" }],
            outputs: [{ state: "done", workType: "story" }],
            worker: "reviewer",
          },
        ],
      },
      recordedAt: "2026-04-16T12:00:00Z",
    },
    type: FACTORY_EVENT_TYPES.runRequest,
  },
];

export const currentNamedFactoryExportResponse = {
  metadata: {
    contractSource: "current-factory-api",
  },
  id: "authored-current-factory",
  name: "semantic-workflow",
  version: defaultSessionFactoryVersion,
  workers: [
    {
      executorProvider: "SCRIPT_WRAP",
      modelProvider: "CODEX",
      name: "reviewer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workstations: [
    {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: [{ state: "done", workType: "story" }],
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "reviewer",
    },
  ],
} satisfies FactoryValue;

/** Session factory GET mock payload (includes version required by session-factory normalization). */
export const currentSessionFactoryExportAPIResponse = {
  ...currentNamedFactoryExportResponse,
  version: defaultSessionFactoryVersion,
} satisfies CurrentFactoryDocument;

export function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

export function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}

export function exportImageFile(): File {
  return new File(
    [toArrayBuffer(fromBase64(onePixelPngBase64))],
    "cover.png",
    { type: "image/png" },
  );
}

export function createDeferredPromise<T>(): {
  promise: Promise<T>;
  reject: (reason?: unknown) => void;
  resolve: (value: T) => void;
} {
  let reject!: (reason?: unknown) => void;
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    reject = rejectPromise;
    resolve = resolvePromise;
  });

  return {
    promise,
    reject,
    resolve,
  };
}

export function installExportDownloadProbe(): {
  getDownloadedBlob: () => Blob | null;
  getDownloadedFilename: () => string;
  restore: () => void;
} {
  class MockExportOffscreenCanvas {
    public constructor(
      public readonly width: number,
      public readonly height: number,
    ) {}

    public getContext(_contextID: "2d"): OffscreenCanvasRenderingContext2D {
      return {
        drawImage() {},
      } as unknown as OffscreenCanvasRenderingContext2D;
    }

    public async convertToBlob(): Promise<Blob> {
      return new Blob([toArrayBuffer(fromBase64(onePixelPngBase64))], {
        type: "image/png",
      });
    }
  }

  const originalCreateObjectURL = window.URL.createObjectURL;
  const originalRevokeObjectURL = window.URL.revokeObjectURL;
  const originalCreateImageBitmap = globalThis.createImageBitmap;
  const originalOffscreenCanvas = globalThis.OffscreenCanvas;
  const originalClick = HTMLAnchorElement.prototype.click;
  let downloadedBlob: Blob | null = null;
  let downloadedFilename = "";

  window.URL.createObjectURL = ((blob: Blob) => {
    downloadedBlob = blob;
    return "blob:app-test-export";
  }) as typeof URL.createObjectURL;
  window.URL.revokeObjectURL = (() => {}) as typeof URL.revokeObjectURL;
  globalThis.createImageBitmap = (async () => ({
    close: () => {},
    height: 1,
    width: 1,
  })) as typeof createImageBitmap;
  globalThis.OffscreenCanvas =
    MockExportOffscreenCanvas as unknown as typeof OffscreenCanvas;
  HTMLAnchorElement.prototype.click = function click(): void {
    downloadedFilename = this.download;
  };

  return {
    getDownloadedBlob: () => downloadedBlob,
    getDownloadedFilename: () => downloadedFilename,
    restore: () => {
      window.URL.createObjectURL = originalCreateObjectURL;
      window.URL.revokeObjectURL = originalRevokeObjectURL;
      globalThis.createImageBitmap = originalCreateImageBitmap;
      globalThis.OffscreenCanvas = originalOffscreenCanvas;
      HTMLAnchorElement.prototype.click = originalClick;
    },
  };
}
