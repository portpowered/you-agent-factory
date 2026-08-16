import type { CSSProperties, Ref } from "react";
import type { ResizeHandleAxis } from "react-grid-layout/core";

export function renderBentoResizeHandle(
  axis: ResizeHandleAxis,
  ref: Ref<HTMLElement>,
) {
  const positionStyle: CSSProperties =
    axis === "e"
      ? { marginTop: -22, right: 0, top: "50%", transform: "rotate(315deg)" }
      : axis === "s"
        ? {
            bottom: 0,
            left: "50%",
            marginLeft: -22,
            transform: "rotate(45deg)",
          }
        : { bottom: 0, right: 0 };

  return (
    <span
      aria-hidden="true"
      className={`react-resizable-handle react-resizable-handle-${axis} z-10 h-11 w-11 touch-none`}
      data-bento-touch-resize-handle={axis}
      ref={ref}
      style={{
        ...positionStyle,
        height: 44,
        touchAction: "none",
        width: 44,
      }}
    />
  );
}
