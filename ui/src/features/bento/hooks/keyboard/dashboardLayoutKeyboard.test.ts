import { describe, expect, it } from "vitest";

import {
  applyDashboardKeyboardLayoutOperation,
  type DashboardKeyboardLayoutMode,
} from "./dashboardLayoutKeyboard";

const columns = 12;

function item(
  id: string,
  values: Partial<{ h: number; w: number; x: number; y: number }> = {},
) {
  return {
    h: values.h ?? 2,
    i: id,
    w: values.w ?? 4,
    x: values.x ?? 0,
    y: values.y ?? 0,
  };
}

function operation(
  mode: DashboardKeyboardLayoutMode,
  key: string,
  layout = [item("activity")],
) {
  return applyDashboardKeyboardLayoutOperation(
    layout,
    "activity",
    mode,
    key,
    columns,
  );
}

describe("applyDashboardKeyboardLayoutOperation", () => {
  it("moves by one grid cell and keeps the layout collision-free", () => {
    const result = operation("move", "ArrowRight", [
      item("activity"),
      item("trace", { x: 4 }),
    ]);

    expect(result.changed).toBe(true);
    expect(result.layout.find((entry) => entry.i === "activity")).toMatchObject(
      { x: 1, y: 0 },
    );
    expect(result.layout.find((entry) => entry.i === "trace")).toMatchObject({
      y: 2,
    });
  });

  it("honors movement boundaries and item size constraints", () => {
    const atLeft = operation("move", "ArrowLeft");
    expect(atLeft.changed).toBe(false);
    expect(atLeft.reason).toBe("boundary");

    const constrained = operation("resize", "ArrowLeft", [
      {
        ...item("activity", { w: 2 }),
        minW: 2,
      },
    ]);
    expect(constrained.changed).toBe(false);
    expect(constrained.reason).toBe("boundary");
  });

  it("resizes by one grid cell without mutating the source layout", () => {
    const source = [item("activity", { w: 4, h: 2 })];
    const result = operation("resize", "ArrowDown", source);

    expect(result.changed).toBe(true);
    expect(result.layout.find((entry) => entry.i === "activity")).toMatchObject(
      { h: 3, w: 4 },
    );
    expect(source[0]).toMatchObject({ h: 2, w: 4 });
  });
});
