import { fireEvent, render, screen } from "@testing-library/react";

import { HorizontalResizableWidth } from "./horizontal-resizable-width";

describe("HorizontalResizableWidth", () => {
  it("exposes a vertical resize separator and updates width while dragging", () => {
    const { container } = render(
      <div style={{ width: "640px" }}>
        <HorizontalResizableWidth resizeHandleLabel="Resize prompt editor width">
          <div data-testid="prompt-surface">Prompt editor</div>
        </HorizontalResizableWidth>
      </div>,
    );

    const resizable = container.querySelector(
      "[data-prompt-editor-resizable='true']",
    ) as HTMLElement;
    const handle = screen.getByRole("slider", {
      name: "Resize prompt editor width",
    });

    Object.defineProperty(resizable, "offsetWidth", {
      configurable: true,
      value: 640,
    });

    fireEvent.pointerDown(handle, { button: 0, clientX: 100, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientX: 180, pointerId: 1 });
    fireEvent.pointerUp(handle, { pointerId: 1 });

    expect(resizable.style.width).toBe("720px");
    expect(handle.getAttribute("aria-valuenow")).toBe("720");
  });

  it("clamps dragged width to configured bounds", () => {
    const { container } = render(
      <div style={{ width: "400px" }}>
        <HorizontalResizableWidth resizeHandleLabel="Resize prompt editor width">
          <div>Prompt editor</div>
        </HorizontalResizableWidth>
      </div>,
    );

    const resizable = container.querySelector(
      "[data-prompt-editor-resizable='true']",
    ) as HTMLElement;
    const handle = screen.getByRole("slider", {
      name: "Resize prompt editor width",
    });

    Object.defineProperty(resizable, "offsetWidth", {
      configurable: true,
      value: 400,
    });

    fireEvent.pointerDown(handle, { button: 0, clientX: 200, pointerId: 2 });
    fireEvent.pointerMove(handle, { clientX: -200, pointerId: 2 });
    fireEvent.pointerUp(handle, { pointerId: 2 });

    expect(resizable.style.width).toBe("280px");
  });
});
