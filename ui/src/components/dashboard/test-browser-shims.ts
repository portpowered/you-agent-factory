const DEFAULT_RECT = {
  bottom: 600,
  height: 600,
  left: 0,
  right: 1000,
  top: 0,
  width: 1000,
  x: 0,
  y: 0,
  toJSON: () => ({}),
};

class DashboardResizeObserver {
  public constructor(private readonly callback: ResizeObserverCallback) {}

  public disconnect(): void {}

  public observe(target: Element): void {
    this.callback(
      [
        {
          borderBoxSize: [],
          contentBoxSize: [],
          contentRect: DEFAULT_RECT,
          devicePixelContentBoxSize: [],
          target,
        } as ResizeObserverEntry,
      ],
      this,
    );
  }

  public unobserve(): void {}
}

class DashboardDOMMatrixReadOnly {
  public readonly m22: number;

  public constructor(transform?: string) {
    const scaleMatch = transform?.match(/scale\(([^)]+)\)/);
    this.m22 = scaleMatch?.[1] ? Number.parseFloat(scaleMatch[1]) : 1;
  }
}

function installAnimationFrameShim() {
  const requestAnimationFrame = globalThis.requestAnimationFrame;
  const cancelAnimationFrame = globalThis.cancelAnimationFrame;
  let nextAnimationFrameHandle = 1;
  const cancelledAnimationFrames = new Set<number>();

  globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) => {
    const handle = nextAnimationFrameHandle++;

    queueMicrotask(() => {
      if (cancelledAnimationFrames.delete(handle)) {
        return;
      }
      callback(performance.now());
    });

    return handle;
  }) as typeof requestAnimationFrame;
  globalThis.cancelAnimationFrame = ((handle: number) => {
    cancelledAnimationFrames.add(handle);
  }) as typeof cancelAnimationFrame;

  return () => {
    globalThis.requestAnimationFrame = requestAnimationFrame;
    globalThis.cancelAnimationFrame = cancelAnimationFrame;
  };
}

export function installLocalStorageShim(): () => void {
  const descriptor = Object.getOwnPropertyDescriptor(
    globalThis,
    "localStorage",
  );
  const values = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return values.size;
    },
    clear() {
      values.clear();
    },
    getItem(key) {
      return values.get(String(key)) ?? null;
    },
    key(index) {
      return [...values.keys()][index] ?? null;
    },
    removeItem(key) {
      values.delete(String(key));
    },
    setItem(key, value) {
      values.set(String(key), String(value));
    },
  };
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: storage,
  });

  return () => {
    if (descriptor) {
      Object.defineProperty(globalThis, "localStorage", descriptor);
    } else {
      Reflect.deleteProperty(globalThis, "localStorage");
    }
  };
}

function installElementMeasurementShims() {
  const resizeObserver = globalThis.ResizeObserver;
  const domMatrixReadOnly = globalThis.DOMMatrixReadOnly;
  const offsetParentDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "offsetParent",
  );
  const boundingRectDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "getBoundingClientRect",
  );
  const offsetWidthDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "offsetWidth",
  );
  const offsetHeightDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "offsetHeight",
  );
  const clientWidthDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "clientWidth",
  );
  const clientHeightDescriptor = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "clientHeight",
  );
  const svgGetBBoxDescriptor = Object.getOwnPropertyDescriptor(
    SVGElement.prototype,
    "getBBox",
  );

  globalThis.ResizeObserver = DashboardResizeObserver;
  globalThis.DOMMatrixReadOnly =
    DashboardDOMMatrixReadOnly as unknown as typeof DOMMatrixReadOnly;
  Object.defineProperty(HTMLElement.prototype, "offsetParent", {
    configurable: true,
    get() {
      return this.parentElement ?? document.body;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "getBoundingClientRect", {
    configurable: true,
    value() {
      return DEFAULT_RECT;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "offsetWidth", {
    configurable: true,
    get() {
      return 1000;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "offsetHeight", {
    configurable: true,
    get() {
      return 600;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get() {
      return 1000;
    },
  });
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get() {
      return 600;
    },
  });
  Object.defineProperty(SVGElement.prototype, "getBBox", {
    configurable: true,
    value() {
      return {
        height: 16,
        width: 120,
        x: 0,
        y: 0,
      };
    },
  });
  const hasPointerCaptureDescriptor = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "hasPointerCapture",
  );
  const setPointerCaptureDescriptor = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "setPointerCapture",
  );
  const releasePointerCaptureDescriptor = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "releasePointerCapture",
  );
  const scrollIntoViewDescriptor = Object.getOwnPropertyDescriptor(
    Element.prototype,
    "scrollIntoView",
  );

  Object.defineProperty(Element.prototype, "hasPointerCapture", {
    configurable: true,
    value() {
      return false;
    },
  });
  Object.defineProperty(Element.prototype, "setPointerCapture", {
    configurable: true,
    value() {},
  });
  Object.defineProperty(Element.prototype, "releasePointerCapture", {
    configurable: true,
    value() {},
  });
  Object.defineProperty(Element.prototype, "scrollIntoView", {
    configurable: true,
    value() {},
  });

  return () => {
    globalThis.ResizeObserver = resizeObserver;
    globalThis.DOMMatrixReadOnly = domMatrixReadOnly;
    if (offsetParentDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "offsetParent",
        offsetParentDescriptor,
      );
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "offsetParent");
    }
    if (boundingRectDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "getBoundingClientRect",
        boundingRectDescriptor,
      );
    }
    if (offsetWidthDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "offsetWidth",
        offsetWidthDescriptor,
      );
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "offsetWidth");
    }
    if (offsetHeightDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "offsetHeight",
        offsetHeightDescriptor,
      );
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "offsetHeight");
    }
    if (clientWidthDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "clientWidth",
        clientWidthDescriptor,
      );
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
    }
    if (clientHeightDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "clientHeight",
        clientHeightDescriptor,
      );
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
    }
    if (svgGetBBoxDescriptor) {
      Object.defineProperty(
        SVGElement.prototype,
        "getBBox",
        svgGetBBoxDescriptor,
      );
    } else {
      Reflect.deleteProperty(SVGElement.prototype, "getBBox");
    }
    if (hasPointerCaptureDescriptor) {
      Object.defineProperty(
        Element.prototype,
        "hasPointerCapture",
        hasPointerCaptureDescriptor,
      );
    } else {
      Reflect.deleteProperty(Element.prototype, "hasPointerCapture");
    }
    if (setPointerCaptureDescriptor) {
      Object.defineProperty(
        Element.prototype,
        "setPointerCapture",
        setPointerCaptureDescriptor,
      );
    } else {
      Reflect.deleteProperty(Element.prototype, "setPointerCapture");
    }
    if (releasePointerCaptureDescriptor) {
      Object.defineProperty(
        Element.prototype,
        "releasePointerCapture",
        releasePointerCaptureDescriptor,
      );
    } else {
      Reflect.deleteProperty(Element.prototype, "releasePointerCapture");
    }
    if (scrollIntoViewDescriptor) {
      Object.defineProperty(
        Element.prototype,
        "scrollIntoView",
        scrollIntoViewDescriptor,
      );
    } else {
      Reflect.deleteProperty(Element.prototype, "scrollIntoView");
    }
  };
}

export function installResizeObserverShim(): () => void {
  const resizeObserver = globalThis.ResizeObserver;
  if (resizeObserver) {
    return () => {};
  }

  globalThis.ResizeObserver =
    DashboardResizeObserver as unknown as typeof ResizeObserver;

  return () => {
    if (resizeObserver) {
      globalThis.ResizeObserver = resizeObserver;
    } else {
      Reflect.deleteProperty(globalThis, "ResizeObserver");
    }
  };
}

export function installDashboardBrowserTestShims(): () => void {
  const restoreLocalStorage = installLocalStorageShim();
  const restoreAnimationFrame = installAnimationFrameShim();
  const restoreElementMeasurements = installElementMeasurementShims();

  return () => {
    restoreAnimationFrame();
    restoreElementMeasurements();
    restoreLocalStorage();
  };
}
