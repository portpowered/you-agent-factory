import "@testing-library/jest-dom/vitest";

const DEFAULT_CHART_RECT = {
  height: 320,
  width: 720,
} as const;

const DEFAULT_LEGEND_RECT = {
  height: 32,
  width: 320,
} as const;

function createDOMRect({
  height,
  left = 0,
  top = 0,
  width,
}: {
  height: number;
  left?: number;
  top?: number;
  width: number;
}): DOMRect {
  return {
    bottom: top + height,
    height,
    left,
    right: left + width,
    top,
    width,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect;
}

function rectWithFallback(rect: DOMRect, fallback: typeof DEFAULT_CHART_RECT) {
  if (rect.width > 0 && rect.height > 0) {
    return rect;
  }

  return createDOMRect({
    height: rect.height > 0 ? rect.height : fallback.height,
    left: rect.left,
    top: rect.top,
    width: rect.width > 0 ? rect.width : fallback.width,
  });
}

class StorybookResizeObserver {
  public constructor(private readonly callback: ResizeObserverCallback) {}

  public disconnect(): void {}

  public observe(target: Element): void {
    const rect =
      target instanceof HTMLElement
        ? rectWithFallback(target.getBoundingClientRect(), DEFAULT_CHART_RECT)
        : createDOMRect(DEFAULT_CHART_RECT);

    this.callback(
      [
        {
          borderBoxSize: [],
          contentBoxSize: [],
          contentRect: rect,
          devicePixelContentBoxSize: [],
          target,
        } as ResizeObserverEntry,
      ],
      this as unknown as ResizeObserver,
    );
  }

  public unobserve(): void {}
}

const originalGetBoundingClientRect =
  HTMLElement.prototype.getBoundingClientRect;

globalThis.ResizeObserver =
  StorybookResizeObserver as unknown as typeof ResizeObserver;

HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
  const rect = originalGetBoundingClientRect.call(this);

  if (this.classList.contains("recharts-responsive-container")) {
    return rectWithFallback(rect, DEFAULT_CHART_RECT);
  }

  if (this.dataset.workChartLegend === "true") {
    return rectWithFallback(rect, DEFAULT_LEGEND_RECT);
  }

  return rect;
};
