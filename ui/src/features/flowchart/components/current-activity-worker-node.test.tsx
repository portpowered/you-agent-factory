import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type CurrentActivityWorkerNode,
  WorkerNodeView,
} from "./current-activity-worker-node";

vi.mock("@xyflow/react", () => ({
  Handle: ({ id }: { id: string }) => <div data-testid={`handle-${id}`} />,
  Position: { Left: "left", Right: "right" },
}));

const workerNodeProps: NodeProps<CurrentActivityWorkerNode> = {
  data: {
    activeFlow: false,
    handles: [],
    kind: "worker",
    muted: false,
    onSelectWorker: vi.fn(),
    place: {
      kind: "constraint",
      place_id: "worker:writer",
      state_value: "writer",
      type_id: "worker",
    },
    selectedWorker: false,
  },
  dragging: false,
  id: "worker:writer",
  isConnectable: false,
  selected: false,
  type: "worker",
  zIndex: 0,
};

function renderWorkerNode() {
  return render(<WorkerNodeView {...workerNodeProps} />);
}

function renderWorkerIcon(overrides: {
  runnerId?: string | null;
  workerType?: string | null;
}) {
  return render(
    <WorkerNodeView
      {...workerNodeProps}
      data={{ ...workerNodeProps.data, ...overrides }}
    />,
  );
}

function bounds(top: number, bottom: number): DOMRect {
  return {
    bottom,
    height: bottom - top,
    left: 0,
    right: 156,
    toJSON: () => ({}),
    top,
    width: 156,
    x: 0,
    y: top,
  } as DOMRect;
}

describe("CurrentActivity worker node layout", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it.each([
    {
      label: bounds(116, 126),
      name: "compact",
      shell: bounds(100, 158),
    },
    {
      label: bounds(366, 376),
      name: "expanded",
      shell: bounds(300, 444),
    },
  ])(
    "keeps the WORKER kind label inside the $name worker node shell",
    ({ label, shell }) => {
      const { container } = renderWorkerNode();
      const nodeShell = container.querySelector(
        "[data-current-activity-node-type='worker']",
      );
      const kindLabel = container.querySelector("[data-worker-kind-label]");

      expect(nodeShell).not.toBeNull();
      expect(kindLabel).not.toBeNull();
      expect(nodeShell?.querySelector("[data-worker-kind-label]")).toBe(
        kindLabel,
      );

      vi.spyOn(
        nodeShell as HTMLElement,
        "getBoundingClientRect",
      ).mockReturnValue(shell);
      vi.spyOn(
        kindLabel as HTMLElement,
        "getBoundingClientRect",
      ).mockReturnValue(label);

      const shellBounds = nodeShell?.getBoundingClientRect();
      const labelBounds = kindLabel?.getBoundingClientRect();

      expect(labelBounds?.top).toBeGreaterThanOrEqual(shellBounds?.top ?? 0);
      expect(labelBounds?.bottom).toBeLessThanOrEqual(shellBounds?.bottom ?? 0);
    },
  );

  it("uses the full selectable content track so centering does not clip the kind label", () => {
    const { container } = renderWorkerNode();
    const button = container.querySelector("button");

    expect(button?.className).toContain("h-full");
    expect(button?.className).toContain("place-content-center");
    expect(
      container.querySelector("[data-worker-label-zone]")?.className,
    ).toContain("h-full");
  });
});

describe("CurrentActivity worker node icons", () => {
  afterEach(() => {
    cleanup();
  });

  it.each([
    ["SCRIPT_WORKER", "codex", "script"],
    [undefined, "codex", "codex"],
    [undefined, "CLAUDE", "claude"],
    [undefined, "antigravity", "antigravity"],
  ])("selects the %s/%s worker glyph", (workerType, runnerId, expectedKind) => {
    const { container } = renderWorkerIcon({ workerType, runnerId });

    expect(
      container
        .querySelector("[data-graph-semantic-icon]")
        ?.getAttribute("data-graph-semantic-icon"),
    ).toBe(expectedKind);
  });

  it.each([undefined, null, "", "future-runner"])(
    "keeps %s runner identity on the generic worker glyph",
    (runnerId) => {
      const { container } = renderWorkerIcon({ runnerId });
      const icon = container.querySelector("[data-graph-semantic-icon]");

      expect(icon?.getAttribute("data-graph-semantic-icon")).toBe("worker");
      expect(icon?.querySelectorAll("path, circle").length).toBeGreaterThan(0);
      expect(icon?.getAttribute("aria-label")).toBe("Worker");
    },
  );
});
