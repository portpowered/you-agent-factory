import { fireEvent, render, screen } from "@testing-library/react";

import { VerticalResizableWidth } from "./vertical-resizable-width";

describe("VerticalResizableWidth", () => {
  it("exposes a vertical resize slider and updates height while dragging", () => {
    const { container } = render(
      <div>
        <VerticalResizableWidth resizeHandleLabel="Resize prompt editor height">
          <div data-testid="prompt-surface">Prompt editor</div>
        </VerticalResizableWidth>
      </div>,
    );

    const resizable = container.querySelector(
      "[data-prompt-editor-resizable='true']",
    ) as HTMLElement;
    const handle = screen.getByRole("slider", {
      name: "Resize prompt editor height",
    });

    Object.defineProperty(resizable, "offsetHeight", {
      configurable: true,
      value: 216,
    });

    fireEvent.pointerDown(handle, { button: 0, clientY: 100, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 180, pointerId: 1 });
    fireEvent.pointerUp(handle, { pointerId: 1 });

    expect(resizable.style.height).toBe("296px");
    expect(handle.getAttribute("aria-orientation")).toBe("vertical");
    expect(handle.getAttribute("aria-valuenow")).toBe("296");
  });

  it("clamps dragged height to configured bounds", () => {
    const { container } = render(
      <div>
        <VerticalResizableWidth resizeHandleLabel="Resize prompt editor height">
          <div>Prompt editor</div>
        </VerticalResizableWidth>
      </div>,
    );

    const resizable = container.querySelector(
      "[data-prompt-editor-resizable='true']",
    ) as HTMLElement;
    const handle = screen.getByRole("slider", {
      name: "Resize prompt editor height",
    });

    Object.defineProperty(resizable, "offsetHeight", {
      configurable: true,
      value: 216,
    });

    fireEvent.pointerDown(handle, { button: 0, clientY: 200, pointerId: 2 });
    fireEvent.pointerMove(handle, { clientY: -200, pointerId: 2 });
    fireEvent.pointerUp(handle, { pointerId: 2 });

    expect(resizable.style.height).toBe("160px");
  });
});
