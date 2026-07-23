import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  canonicalLayoutViewportKey,
  useCanonicalLayoutViewportSync,
} from "./use-canonical-layout-viewport-sync";

describe("canonicalLayoutViewportKey", () => {
  it("returns a stable key for saved viewport values", () => {
    expect(canonicalLayoutViewportKey({ x: 120, y: 80, zoom: 1.25 })).toBe(
      "120:80:1.25",
    );
  });

  it("returns none when viewport metadata is absent", () => {
    expect(canonicalLayoutViewportKey(undefined)).toBe("none");
    expect(canonicalLayoutViewportKey(null)).toBe("none");
  });
});

describe("useCanonicalLayoutViewportSync", () => {
  it("applies saved viewport when canonical metadata is present", () => {
    const setViewport = vi.fn().mockResolvedValue(undefined);
    const fitView = vi.fn().mockResolvedValue(undefined);
    const flowInstanceRef = { current: { setViewport, fitView } };
    const skipNextViewportMoveEndRef = { current: false };

    renderHook(() =>
      useCanonicalLayoutViewportSync({
        canonicalLayoutViewport: { x: 10, y: 20, zoom: 1.5 },
        fitViewOptions: { padding: 0.2 },
        flowInstanceRef,
        skipNextViewportMoveEndRef,
        viewportResetKey: "base",
      }),
    );

    expect(setViewport).toHaveBeenCalledWith(
      { x: 10, y: 20, zoom: 1.5 },
      { duration: 0 },
    );
    expect(fitView).not.toHaveBeenCalled();
    expect(skipNextViewportMoveEndRef.current).toBe(true);
  });

  it("falls back to fitView when canonical viewport is absent", () => {
    const setViewport = vi.fn().mockResolvedValue(undefined);
    const fitView = vi.fn().mockResolvedValue(undefined);
    const flowInstanceRef = { current: { setViewport, fitView } };
    const fitViewOptions = { padding: 0.2 };
    const skipNextViewportMoveEndRef = { current: false };

    renderHook(() =>
      useCanonicalLayoutViewportSync({
        canonicalLayoutViewport: null,
        fitViewOptions,
        flowInstanceRef,
        skipNextViewportMoveEndRef,
        viewportResetKey: "base",
      }),
    );

    expect(fitView).toHaveBeenCalledWith(fitViewOptions);
    expect(setViewport).not.toHaveBeenCalled();
    expect(skipNextViewportMoveEndRef.current).toBe(true);
  });

  it("skips when flow instance is unavailable", () => {
    const setViewport = vi.fn();
    const fitView = vi.fn();
    const flowInstanceRef = { current: null };

    renderHook(() =>
      useCanonicalLayoutViewportSync({
        canonicalLayoutViewport: { x: 0, y: 0, zoom: 1 },
        fitViewOptions: {},
        flowInstanceRef,
        viewportResetKey: "base",
      }),
    );

    expect(setViewport).not.toHaveBeenCalled();
    expect(fitView).not.toHaveBeenCalled();
  });

  it("does not reapply viewport when reset key and viewport are unchanged", () => {
    const setViewport = vi.fn().mockResolvedValue(undefined);
    const fitView = vi.fn().mockResolvedValue(undefined);
    const flowInstanceRef = { current: { setViewport, fitView } };
    const props = {
      canonicalLayoutViewport: { x: 5, y: 5, zoom: 2 },
      fitViewOptions: { padding: 0.1 },
      flowInstanceRef,
      viewportResetKey: "stable",
    };

    const { rerender } = renderHook(
      (input) => useCanonicalLayoutViewportSync(input),
      { initialProps: props },
    );

    expect(setViewport).toHaveBeenCalledTimes(1);

    rerender({ ...props });
    expect(setViewport).toHaveBeenCalledTimes(1);
  });

  it("reapplies viewport when reset key changes", () => {
    const setViewport = vi.fn().mockResolvedValue(undefined);
    const fitView = vi.fn().mockResolvedValue(undefined);
    const flowInstanceRef = { current: { setViewport, fitView } };

    const { rerender } = renderHook(
      ({ viewportResetKey }) =>
        useCanonicalLayoutViewportSync({
          canonicalLayoutViewport: { x: 1, y: 2, zoom: 1 },
          fitViewOptions: {},
          flowInstanceRef,
          viewportResetKey,
        }),
      { initialProps: { viewportResetKey: "a" } },
    );

    expect(setViewport).toHaveBeenCalledTimes(1);

    rerender({ viewportResetKey: "b" });
    expect(setViewport).toHaveBeenCalledTimes(2);
  });
});
