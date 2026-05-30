/**
 * Bun unit/coverage lane: register jsdom globals before any test or Testing Library import.
 * Do not import @testing-library/react here — ES module evaluation order requires document
 * to exist before @testing-library/dom initializes screen queries.
 */
if (process.env.BUN_TEST_NODE_LANE !== "1") {
  const { JSDOM } = await import("jsdom");

  const dom = new JSDOM("<!DOCTYPE html><html><body></body></html>", {
    url: "http://localhost/",
  });

  const { window } = dom;

  const globalScope = globalThis as typeof globalThis & {
    window: Window & typeof globalThis;
    document: Document;
    navigator: Navigator;
    HTMLElement: typeof HTMLElement;
    SVGElement: typeof SVGElement;
    Element: typeof Element;
    Node: typeof Node;
    Text: typeof Text;
    DocumentFragment: typeof DocumentFragment;
    Event: typeof Event;
    CustomEvent: typeof CustomEvent;
    MouseEvent: typeof MouseEvent;
    KeyboardEvent: typeof KeyboardEvent;
    FocusEvent: typeof FocusEvent;
    InputEvent: typeof InputEvent;
    getComputedStyle: typeof window.getComputedStyle;
    requestAnimationFrame?: typeof window.requestAnimationFrame;
    cancelAnimationFrame?: typeof window.cancelAnimationFrame;
  };

  globalScope.window = window as Window & typeof globalThis;
  globalScope.document = window.document;
  globalScope.navigator = window.navigator;
  globalScope.HTMLElement = window.HTMLElement;
  globalScope.SVGElement = window.SVGElement;
  globalScope.Element = window.Element;
  globalScope.Node = window.Node;
  globalScope.Text = window.Text;
  globalScope.DocumentFragment = window.DocumentFragment;
  globalScope.Event = window.Event;
  globalScope.CustomEvent = window.CustomEvent;
  globalScope.MouseEvent = window.MouseEvent;
  globalScope.KeyboardEvent = window.KeyboardEvent;
  globalScope.FocusEvent = window.FocusEvent;
  globalScope.InputEvent = window.InputEvent;
  globalScope.getComputedStyle = window.getComputedStyle.bind(window);

  const windowScope = window as Window & typeof globalThis & Record<string, unknown>;
  for (const key of [
    "HTMLAnchorElement",
    "HTMLButtonElement",
    "HTMLDivElement",
    "HTMLFormElement",
    "HTMLInputElement",
    "HTMLLabelElement",
    "HTMLSelectElement",
    "HTMLTextAreaElement",
    "HTMLDialogElement",
    "HTMLImageElement",
    "HTMLIFrameElement",
    "HTMLCanvasElement",
    "MutationObserver",
    "ResizeObserver",
    "IntersectionObserver",
    "DOMMatrixReadOnly",
    "DOMMatrix",
    "DOMRect",
    "DOMRectReadOnly",
    "NodeFilter",
    "TreeWalker",
    "Range",
    "Selection",
    "CSS",
    "URL",
    "URLSearchParams",
    "Blob",
    "File",
    "FileReader",
    "FormData",
    "Headers",
    "Request",
    "Response",
    "AbortController",
    "AbortSignal",
    "localStorage",
    "sessionStorage",
    "matchMedia",
    "atob",
    "btoa",
    "structuredClone",
  ] as const) {
    if (key in windowScope && windowScope[key] !== undefined) {
      (globalScope as Record<string, unknown>)[key] = windowScope[key];
    }
  }

  if (typeof globalScope.matchMedia !== "function") {
    globalScope.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
  }

  if (typeof window.requestAnimationFrame === "function") {
    globalScope.requestAnimationFrame = window.requestAnimationFrame.bind(window);
    globalScope.cancelAnimationFrame = window.cancelAnimationFrame.bind(window);
  } else {
    globalScope.requestAnimationFrame = (callback: FrameRequestCallback) =>
      setTimeout(() => callback(Date.now()), 0) as unknown as number;
    globalScope.cancelAnimationFrame = (handle: number) => {
      clearTimeout(handle);
    };
  }

  if (typeof Blob !== "undefined" && typeof Blob.prototype.arrayBuffer !== "function") {
    Blob.prototype.arrayBuffer = function arrayBuffer() {
      return new Promise<ArrayBuffer>((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
          if (reader.result instanceof ArrayBuffer) {
            resolve(reader.result);
            return;
          }
          reject(new Error("FileReader did not return an ArrayBuffer."));
        };
        reader.onerror = () => {
          reject(reader.error ?? new Error("FileReader failed."));
        };
        reader.readAsArrayBuffer(this);
      });
    };
  }

  const objectUrlBlobs = new Map<string, Blob>();
  let nextObjectUrlId = 0;
  const createObjectURL = (blob: Blob) => {
    const url = `blob:http://localhost/${nextObjectUrlId++}`;
    objectUrlBlobs.set(url, blob);
    return url;
  };
  const revokeObjectURL = (url: string) => {
    objectUrlBlobs.delete(url);
  };

  globalScope.URL.createObjectURL = createObjectURL;
  globalScope.URL.revokeObjectURL = revokeObjectURL;
  windowScope.URL.createObjectURL = createObjectURL;
  windowScope.URL.revokeObjectURL = revokeObjectURL;

  const decodeBase64 = (input: string) => Buffer.from(input, "base64").toString("latin1");
  const encodeBase64 = (input: string) => Buffer.from(input, "latin1").toString("base64");
  globalScope.atob = decodeBase64;
  globalScope.btoa = encodeBase64;
  windowScope.atob = decodeBase64;
  windowScope.btoa = encodeBase64;

  const nativeGetBoundingClientRect = Element.prototype.getBoundingClientRect;
  Element.prototype.getBoundingClientRect = function getBoundingClientRect(this: Element) {
    if (
      this instanceof window.HTMLElement &&
      this.offsetWidth === 0 &&
      this.offsetHeight === 0
    ) {
      return new window.DOMRect(0, 0, 0, 0);
    }
    return nativeGetBoundingClientRect.call(this);
  };
}
