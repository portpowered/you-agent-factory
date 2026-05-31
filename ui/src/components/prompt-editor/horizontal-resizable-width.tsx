import {
  type ReactNode,
  type PointerEvent as ReactPointerEvent,
  useCallback,
  useRef,
  useState,
} from "react";
import { cn } from "../../lib/cn";
import {
  clampPromptEditorResizeWidth,
  PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
  PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
  resolvePromptEditorResizeMaxWidth,
} from "./prompt-editor-resize-bounds";

interface HorizontalResizableWidthProps {
  children: ReactNode;
  className?: string;
  maxWidth?: number;
  minWidth?: number;
  resizeHandleLabel: string;
}

export function HorizontalResizableWidth({
  children,
  className,
  maxWidth = PROMPT_EDITOR_RESIZE_MAX_WIDTH_PX,
  minWidth = PROMPT_EDITOR_RESIZE_MIN_WIDTH_PX,
  resizeHandleLabel,
}: HorizontalResizableWidthProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [widthPx, setWidthPx] = useState<number | null>(null);

  const resolveBounds = useCallback(() => {
    const parentWidth =
      containerRef.current?.parentElement?.clientWidth ??
      containerRef.current?.offsetWidth ??
      maxWidth;

    return {
      maxWidth: resolvePromptEditorResizeMaxWidth(maxWidth, parentWidth),
      minWidth,
    };
  }, [maxWidth, minWidth]);

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
      const startWidth = widthPx ?? container.offsetWidth;
      if (widthPx === null) {
        setWidthPx(startWidth);
      }

      const startX = event.clientX;
      const handle = event.currentTarget;
      if (typeof handle.setPointerCapture === "function") {
        handle.setPointerCapture(event.pointerId);
      }

      const handlePointerMove = (moveEvent: PointerEvent) => {
        const nextWidth = clampPromptEditorResizeWidth(
          startWidth + (moveEvent.clientX - startX),
          bounds,
        );
        setWidthPx(nextWidth);
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
    [resolveBounds, widthPx],
  );

  const resolvedWidth = widthPx ?? "100%";
  const ariaValueNow =
    typeof resolvedWidth === "number" ? resolvedWidth : undefined;
  const bounds = resolveBounds();

  return (
    <div
      className={cn("relative max-w-full min-w-0", className)}
      data-prompt-editor-resizable="true"
      ref={containerRef}
      style={{
        width:
          typeof resolvedWidth === "number"
            ? `${resolvedWidth}px`
            : resolvedWidth,
      }}
    >
      {children}
      <div
        aria-label={resizeHandleLabel}
        aria-valuemax={bounds.maxWidth}
        aria-valuemin={bounds.minWidth}
        aria-valuenow={ariaValueNow}
        className="absolute inset-y-0 right-0 z-10 flex w-3 translate-x-1/2 cursor-col-resize touch-none items-stretch justify-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring"
        data-prompt-editor-resize-handle="true"
        onPointerDown={handleResizePointerDown}
        role="slider"
        tabIndex={0}
      >
        <span
          aria-hidden="true"
          className="my-2 w-px rounded-full bg-af-border transition-colors hover:bg-af-border-strong"
        />
      </div>
    </div>
  );
}
