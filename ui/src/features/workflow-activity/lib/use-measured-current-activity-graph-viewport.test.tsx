import { act, render, screen } from "@testing-library/react";
import { useRef } from "react";

import { useMeasuredCurrentActivityGraphViewport } from "./use-measured-current-activity-graph-viewport";

function HookProbe() {
  const graphViewportRef = useRef<HTMLElement | null>(null);
  const graphViewportReady =
    useMeasuredCurrentActivityGraphViewport(graphViewportRef);

  return (
    <section
      data-height={String(graphViewportReady.height)}
      aria-label="Measured graph viewport"
      data-ready={String(graphViewportReady.ready)}
      data-width={String(graphViewportReady.width)}
      ref={graphViewportRef}
    />
  );
}

const originalBoundingClientRectDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "getBoundingClientRect",
);
const originalResizeObserver = globalThis.ResizeObserver;
const originalNavigatorDescriptor = Object.getOwnPropertyDescriptor(
  window.navigator,
  "userAgent",
);
const originalRequestAnimationFrame = globalThis.requestAnimationFrame;
const originalCancelAnimationFrame = globalThis.cancelAnimationFrame;

function setBoundingClientRect(
  value: typeof HTMLElement.prototype.getBoundingClientRect,
) {
  Object.defineProperty(HTMLElement.prototype, "getBoundingClientRect", {
    configurable: true,
    value,
  });
}

function restoreBrowserGlobals() {
  globalThis.ResizeObserver = originalResizeObserver;
  globalThis.requestAnimationFrame = originalRequestAnimationFrame;
  globalThis.cancelAnimationFrame = originalCancelAnimationFrame;
  if (originalNavigatorDescriptor) {
    Object.defineProperty(
      window.navigator,
      "userAgent",
      originalNavigatorDescriptor,
    );
  }
  if (originalBoundingClientRectDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      "getBoundingClientRect",
      originalBoundingClientRectDescriptor,
    );
  }
}

describe("useMeasuredCurrentActivityGraphViewport", () => {
  afterEach(restoreBrowserGlobals);

  it("waits for non-zero viewport dimensions before marking the graph ready", () => {
    let resizeCallback: ResizeObserverCallback | undefined;

    class ResizeObserverMock {
      public constructor(callback: ResizeObserverCallback) {
        resizeCallback = callback;
      }

      public disconnect(): void {}

      public observe(): void {}

      public unobserve(): void {}
    }

    Object.defineProperty(window.navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0",
    });
    globalThis.ResizeObserver =
      ResizeObserverMock as unknown as typeof ResizeObserver;
    setBoundingClientRect(
      () =>
        ({
          bottom: 0,
          height: 0,
          left: 0,
          right: 0,
          top: 0,
          width: 0,
          x: 0,
          y: 0,
          toJSON: () => ({}),
        }) as DOMRect,
    );

    render(<HookProbe />);

    const probe = screen.getByRole("region", {
      name: "Measured graph viewport",
    });
    expect(probe.getAttribute("data-ready")).toBe("false");

    act(() => {
      resizeCallback?.(
        [
          {
            contentRect: {
              bottom: 480,
              height: 480,
              left: 0,
              right: 640,
              top: 0,
              width: 640,
              x: 0,
              y: 0,
              toJSON: () => ({}),
            },
          } as ResizeObserverEntry,
        ],
        {} as ResizeObserver,
      );
    });

    expect(probe.getAttribute("data-ready")).toBe("true");
    expect(probe.getAttribute("data-width")).toBe("640");
    expect(probe.getAttribute("data-height")).toBe("480");
  });

  it("retries on animation frames until the viewport measures non-zero", () => {
    let requestAnimationFrameCallback: FrameRequestCallback | undefined;
    let measureCount = 0;

    Object.defineProperty(window.navigator, "userAgent", {
      configurable: true,
      value: "Mozilla/5.0",
    });
    globalThis.ResizeObserver = undefined;
    globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      requestAnimationFrameCallback = callback;
      return 1;
    }) as typeof requestAnimationFrame;
    globalThis.cancelAnimationFrame = vi.fn();
    setBoundingClientRect(() => {
      measureCount += 1;
      const width = measureCount >= 2 ? 640 : 0;
      const height = measureCount >= 2 ? 480 : 0;

      return {
        bottom: height,
        height,
        left: 0,
        right: width,
        top: 0,
        width,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      } as DOMRect;
    });

    render(<HookProbe />);

    const probe = screen.getByRole("region", {
      name: "Measured graph viewport",
    });
    expect(probe.getAttribute("data-ready")).toBe("false");

    act(() => {
      requestAnimationFrameCallback?.(16);
    });

    expect(probe.getAttribute("data-ready")).toBe("true");
    expect(probe.getAttribute("data-width")).toBe("640");
    expect(probe.getAttribute("data-height")).toBe("480");
  });
});
