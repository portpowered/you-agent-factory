import {
  type ReactNode,
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useRef,
  useState,
} from "react";
import { cn } from "../../lib/cn";
import {
  clampPromptEditorResizeHeight,
  PROMPT_EDITOR_RESIZE_DEFAULT_HEIGHT,
  PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
  PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
} from "./prompt-editor-resize-bounds";

interface VerticalResizableWidthProps {
  children: ReactNode;
  className?: string;
  defaultHeight?: string;
  maxHeight?: number;
  minHeight?: number;
  resizeHandleLabel: string;
}

export function VerticalResizableWidth({
  children,
  className,
  defaultHeight = PROMPT_EDITOR_RESIZE_DEFAULT_HEIGHT,
  maxHeight = PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
  minHeight = PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
  resizeHandleLabel,
}: VerticalResizableWidthProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [heightPx, setHeightPx] = useState<number | null>(null);

  const resolveBounds = useCallback(() => {
    return {
      maxHeight,
      minHeight,
    };
  }, [maxHeight, minHeight]);

  const handleResizePointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (event.button !== 0) {
        return;
      }

      event.preventDefault();
      const container = containerRef.current;
      if (!container) {
        return;
      }

      const bounds = resolveBounds();
      const startHeight = heightPx ?? container.offsetHeight;
      if (heightPx === null) {
        setHeightPx(startHeight);
      }

      const startY = event.clientY;
      const handle = event.currentTarget;
      if (typeof handle.setPointerCapture === "function") {
        handle.setPointerCapture(event.pointerId);
      }

      const handlePointerMove = (moveEvent: PointerEvent) => {
        const nextHeight = clampPromptEditorResizeHeight(
          startHeight + (moveEvent.clientY - startY),
          bounds,
        );
        setHeightPx(nextHeight);
      };

      const endResize = () => {
        if (typeof handle.releasePointerCapture === "function") {
          handle.releasePointerCapture(event.pointerId);
        }
        handle.removeEventListener("pointermove", handlePointerMove);
        handle.removeEventListener("pointerup", endResize);
        handle.removeEventListener("pointercancel", endResize);
      };

      handle.addEventListener("pointermove", handlePointerMove);
      handle.addEventListener("pointerup", endResize);
      handle.addEventListener("pointercancel", endResize);
    },
    [heightPx, resolveBounds],
  );

  const resolvedHeight = heightPx ?? defaultHeight;
  const ariaValueNow =
    typeof resolvedHeight === "number" ? resolvedHeight : undefined;
  const bounds = resolveBounds();

  return (
    <div
      className={cn("relative min-h-0 min-w-0", className)}
      data-prompt-editor-resizable="true"
      ref={containerRef}
      style={{
        height:
          typeof resolvedHeight === "number"
            ? `${resolvedHeight}px`
            : resolvedHeight,
      }}
    >
      {children}
      <div
        aria-label={resizeHandleLabel}
        aria-orientation="vertical"
        aria-valuemax={bounds.maxHeight}
        aria-valuemin={bounds.minHeight}
        aria-valuenow={ariaValueNow}
        className="absolute inset-x-0 bottom-0 z-10 flex h-3 translate-y-1/2 cursor-row-resize touch-none items-center justify-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring"
        data-prompt-editor-resize-handle="true"
        onPointerDown={handleResizePointerDown}
        role="slider"
        tabIndex={0}
      >
        <span
          aria-hidden="true"
          className="mx-2 h-px flex-1 rounded-full bg-outline transition-colors hover:bg-outline-variant"
        />
      </div>
    </div>
  );
}
