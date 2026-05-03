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

export function installDashboardBrowserTestShims(): () => void {
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

  return () => {
    globalThis.ResizeObserver = resizeObserver;
    globalThis.DOMMatrixReadOnly = domMatrixReadOnly;
    if (offsetParentDescriptor) {
      Object.defineProperty(HTMLElement.prototype, "offsetParent", offsetParentDescriptor);
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
      Object.defineProperty(HTMLElement.prototype, "offsetWidth", offsetWidthDescriptor);
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "offsetWidth");
    }
    if (offsetHeightDescriptor) {
      Object.defineProperty(HTMLElement.prototype, "offsetHeight", offsetHeightDescriptor);
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "offsetHeight");
    }
    if (clientWidthDescriptor) {
      Object.defineProperty(HTMLElement.prototype, "clientWidth", clientWidthDescriptor);
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
    }
    if (clientHeightDescriptor) {
      Object.defineProperty(HTMLElement.prototype, "clientHeight", clientHeightDescriptor);
    } else {
      Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
    }
    if (svgGetBBoxDescriptor) {
      Object.defineProperty(SVGElement.prototype, "getBBox", svgGetBBoxDescriptor);
    } else {
      Reflect.deleteProperty(SVGElement.prototype, "getBBox");
    }
  };
}

