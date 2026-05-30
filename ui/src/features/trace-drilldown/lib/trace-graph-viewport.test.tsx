import { act, render, renderHook, screen } from "@testing-library/react";

import { failOnTraceReactFlowError } from "./trace-react-flow-error";
import { useMeasuredTraceGraphViewport } from "./use-measured-trace-graph-viewport";

class ControlledResizeObserver {
  public static instances: ControlledResizeObserver[] = [];

  public constructor(private readonly callback: ResizeObserverCallback) {
    ControlledResizeObserver.instances.push(this);
  }

  public disconnect(): void {}

  public observe(target: Element): void {
    this.resize(target, 0, 0);
  }

  public unobserve(): void {}

  public emitEmptyEntries(): void {
    this.callback([], this);
  }

  public resize(target: Element, width: number, height: number): void {
    this.callback(
      [
        {
          borderBoxSize: [],
          contentBoxSize: [],
          contentRect: {
            bottom: height,
            height,
            left: 0,
            right: width,
            top: 0,
            width,
            x: 0,
            y: 0,
            toJSON: () => ({}),
          },
          devicePixelContentBoxSize: [],
          target,
        } as ResizeObserverEntry,
      ],
      this,
    );
  }
}

function MeasuredViewportProbe() {
  const { graphViewportReady, graphViewportRef } =
    useMeasuredTraceGraphViewport();

  return (
    <div
      data-ready={graphViewportReady ? "ready" : "waiting"}
      data-testid="measured-viewport"
      ref={graphViewportRef}
    />
  );
}

describe("trace graph React Flow errors", () => {
  it("throws when React Flow reports a trace renderer error", () => {
    expect(() => {
      failOnTraceReactFlowError("004", "missing dimensions");
    }).toThrow("Trace React Flow error 004: missing dimensions");
  });
});

describe("useMeasuredTraceGraphViewport", () => {
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    ControlledResizeObserver.instances = [];
    globalThis.ResizeObserver =
      ControlledResizeObserver as unknown as typeof ResizeObserver;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.ResizeObserver = originalResizeObserver;
    vi.restoreAllMocks();
  });

  it("marks the viewport ready only after it has measured dimensions", () => {
    render(<MeasuredViewportProbe />);

    const viewport = screen.getByTestId("measured-viewport");
    expect(viewport.getAttribute("data-ready")).toBe("waiting");
    expect(ControlledResizeObserver.instances).toHaveLength(1);

    act(() => {
      ControlledResizeObserver.instances[0]?.resize(viewport, 640, 360);
    });

    expect(viewport.getAttribute("data-ready")).toBe("ready");
  });

  it("falls back to direct measurement when an observer entry is unavailable", () => {
    render(<MeasuredViewportProbe />);

    const viewport = screen.getByTestId("measured-viewport");
    vi.spyOn(viewport, "getBoundingClientRect").mockReturnValue({
      bottom: 360,
      height: 360,
      left: 0,
      right: 640,
      top: 0,
      width: 640,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);

    act(() => {
      ControlledResizeObserver.instances[0]?.emitEmptyEntries();
    });

    expect(viewport.getAttribute("data-ready")).toBe("ready");
  });

  it("leaves readiness false when no viewport has mounted yet", () => {
    const { result } = renderHook(() => useMeasuredTraceGraphViewport());

    expect(result.current.graphViewportReady).toBe(false);
  });

  it("falls back to animation-frame measurement without ResizeObserver", () => {
    let scheduledFrame: FrameRequestCallback | undefined;
    const cancelAnimationFrame = vi.fn();
    vi.stubGlobal("ResizeObserver", undefined);
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      scheduledFrame = callback;
      return 7;
    });
    vi.stubGlobal("cancelAnimationFrame", cancelAnimationFrame);

    const { unmount } = render(<MeasuredViewportProbe />);
    const viewport = screen.getByTestId("measured-viewport");

    expect(scheduledFrame).toBeDefined();

    vi.spyOn(viewport, "getBoundingClientRect").mockReturnValue({
      bottom: 240,
      height: 240,
      left: 0,
      right: 480,
      top: 0,
      width: 480,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    } as DOMRect);

    act(() => {
      scheduledFrame?.(performance.now());
    });

    expect(viewport.getAttribute("data-ready")).toBe("ready");

    unmount();

    expect(cancelAnimationFrame).toHaveBeenCalledWith(7);
  });
});
