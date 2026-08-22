import { cleanup, render } from "@testing-library/react";
import type { NodeProps } from "@xyflow/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  type CurrentActivityWorkerNode,
  WorkerNodeView,
} from "../current-activity-worker-node";

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

function renderWorkerNode(
  overrides: Partial<CurrentActivityWorkerNode["data"]> = {},
) {
  return render(
    <WorkerNodeView
      {...workerNodeProps}
      data={{ ...workerNodeProps.data, ...overrides }}
    />,
  );
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

  it("renders worker execution details after resize", () => {
    const { container } = renderWorkerNode({
      expanded: true,
      runnerId: "codex",
      workerType: "MODEL_WORKER",
    });

    expect(
      container.querySelector('[data-factory-graph-expanded-content="worker"]'),
    ).toBeTruthy();
    expect(
      container.querySelector(
        '[data-factory-graph-expanded-field="worker-type"]',
      )?.textContent,
    ).toBe("MODEL_WORKER");
    expect(
      container.querySelector('[data-factory-graph-expanded-field="runner"]')
        ?.textContent,
    ).toBe("codex");
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

  it.each([
    {
      activeFlow: false,
      iconClassName: "text-info",
      name: "quiet",
      surfaceClassName: "bg-info-container",
      surface: "quiet",
    },
    {
      activeFlow: true,
      iconClassName: "text-warning",
      name: "active",
      surfaceClassName: "!bg-warning",
      surface: "active",
    },
  ] as const)(
    "keeps the $name worker icon on its node backing tone",
    ({ activeFlow, iconClassName, surface, surfaceClassName }) => {
      const { container } = renderWorkerNode({
        activeFlow,
        focused: true,
        muted: true,
        selectedWorker: true,
        validationError: true,
      });
      const shell = container.querySelector(
        "[data-current-activity-node-type='worker']",
      );
      const icon = container.querySelector("[data-graph-semantic-icon]");

      expect(shell?.getAttribute("data-graph-visual-surface")).toBe(surface);
      expect(shell?.className).toContain(surfaceClassName);
      expect(icon?.getAttribute("class")).toContain(iconClassName);
    },
  );

  it("keeps an unfamiliar worker kind as an accessible raw neutral label", () => {
    const { container, getByText } = renderWorkerNode({
      workerType: "FUTURE_WORKER_KIND",
    });
    const shell = container.querySelector(
      "[data-current-activity-node-type='worker']",
    );
    const kindLabel = getByText("FUTURE_WORKER_KIND");

    expect(kindLabel.getAttribute("data-worker-kind-label")).toBe("true");
    expect(kindLabel.className).toContain("text-on-surface-variant");
    expect(shell?.className).toContain("border-outline bg-surface");
    expect(
      container
        .querySelector("[data-worker-label-zone]")
        ?.getAttribute("aria-label"),
    ).toBe("worker:writer (FUTURE_WORKER_KIND)");
  });
});
