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

class PackageResizeObserver {
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

class PackageDOMMatrixReadOnly {
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

function definePropertyWithRestore<T extends object>(
  target: T,
  property: PropertyKey,
  descriptor: PropertyDescriptor,
) {
  const originalDescriptor = Object.getOwnPropertyDescriptor(target, property);

  Object.defineProperty(target, property, descriptor);

  return () => {
    if (originalDescriptor) {
      Object.defineProperty(target, property, originalDescriptor);
      return;
    }

    Reflect.deleteProperty(target, property);
  };
}

function installGlobalMeasurementShims() {
  const resizeObserver = globalThis.ResizeObserver;
  const domMatrixReadOnly = globalThis.DOMMatrixReadOnly;

  globalThis.ResizeObserver =
    PackageResizeObserver as unknown as typeof ResizeObserver;
  globalThis.DOMMatrixReadOnly =
    PackageDOMMatrixReadOnly as unknown as typeof DOMMatrixReadOnly;

  return () => {
    globalThis.ResizeObserver = resizeObserver;
    globalThis.DOMMatrixReadOnly = domMatrixReadOnly;
  };
}

function installHTMLElementMeasurementShims() {
  const restoreOffsetParent = definePropertyWithRestore(
    HTMLElement.prototype,
    "offsetParent",
    {
      configurable: true,
      get(this: HTMLElement) {
        return this.parentElement ?? document.body;
      },
    },
  );
  const restoreBoundingClientRect = definePropertyWithRestore(
    HTMLElement.prototype,
    "getBoundingClientRect",
    {
      configurable: true,
      value() {
        return DEFAULT_RECT;
      },
    },
  );
  const restoreOffsetWidth = definePropertyWithRestore(
    HTMLElement.prototype,
    "offsetWidth",
    {
      configurable: true,
      get() {
        return 1000;
      },
    },
  );
  const restoreOffsetHeight = definePropertyWithRestore(
    HTMLElement.prototype,
    "offsetHeight",
    {
      configurable: true,
      get() {
        return 600;
      },
    },
  );
  const restoreClientWidth = definePropertyWithRestore(
    HTMLElement.prototype,
    "clientWidth",
    {
      configurable: true,
      get() {
        return 1000;
      },
    },
  );
  const restoreClientHeight = definePropertyWithRestore(
    HTMLElement.prototype,
    "clientHeight",
    {
      configurable: true,
      get() {
        return 600;
      },
    },
  );
  const restoreSvgGetBBox = definePropertyWithRestore(
    SVGElement.prototype,
    "getBBox",
    {
      configurable: true,
      value() {
        return {
          height: 16,
          width: 120,
          x: 0,
          y: 0,
        };
      },
    },
  );

  return () => {
    restoreOffsetParent();
    restoreBoundingClientRect();
    restoreOffsetWidth();
    restoreOffsetHeight();
    restoreClientWidth();
    restoreClientHeight();
    restoreSvgGetBBox();
  };
}

function installPointerInteractionShims() {
  const restoreHasPointerCapture = definePropertyWithRestore(
    Element.prototype,
    "hasPointerCapture",
    {
      configurable: true,
      value() {
        return false;
      },
    },
  );
  const restoreSetPointerCapture = definePropertyWithRestore(
    Element.prototype,
    "setPointerCapture",
    {
      configurable: true,
      value() {},
    },
  );
  const restoreReleasePointerCapture = definePropertyWithRestore(
    Element.prototype,
    "releasePointerCapture",
    {
      configurable: true,
      value() {},
    },
  );
  const restoreScrollIntoView = definePropertyWithRestore(
    Element.prototype,
    "scrollIntoView",
    {
      configurable: true,
      value() {},
    },
  );

  return () => {
    restoreHasPointerCapture();
    restoreSetPointerCapture();
    restoreReleasePointerCapture();
    restoreScrollIntoView();
  };
}

function installElementMeasurementShims() {
  const restoreGlobalMeasurementShims = installGlobalMeasurementShims();
  const restoreHTMLElementMeasurementShims =
    installHTMLElementMeasurementShims();
  const restorePointerInteractionShims = installPointerInteractionShims();

  return () => {
    restoreGlobalMeasurementShims();
    restoreHTMLElementMeasurementShims();
    restorePointerInteractionShims();
  };
}

export function installPackageBrowserTestShims(): () => void {
  const restoreAnimationFrame = installAnimationFrameShim();
  const restoreElementMeasurements = installElementMeasurementShims();

  return () => {
    restoreAnimationFrame();
    restoreElementMeasurements();
  };
}
