// @vitest-environment happy-dom

import { ReactFlowProvider } from "@xyflow/react";
import { describe, expect, it } from "vitest";
import { renderPackageComponent } from "../testing/render";
import type { GraphNodeHandle } from "./graph-node-handle";
import { GraphNodeShell } from "./graph-node-shell";

function renderCompactShell(handles: GraphNodeHandle[]) {
  return renderPackageComponent(
    <ReactFlowProvider>
      <GraphNodeShell contentInset="compact" handles={handles}>
        <span>Node content</span>
      </GraphNodeShell>
    </ReactFlowProvider>,
  );
}

const leftHandle: GraphNodeHandle = {
  id: "left-target",
  label: "Left",
  side: "left",
  type: "target",
};
const rightHandle: GraphNodeHandle = {
  id: "right-source",
  label: "Right",
  side: "right",
  type: "source",
};

describe("GraphNodeShell compact content insets", () => {
  it.each([
    { expected: ["px-2"], handles: [], name: "no handle rail" },
    {
      expected: ["pl-5", "pr-2"],
      handles: [leftHandle],
      name: "left handle rail",
    },
    {
      expected: ["pl-2", "pr-5"],
      handles: [rightHandle],
      name: "right handle rail",
    },
    {
      expected: ["pl-5", "pr-5"],
      handles: [leftHandle, rightHandle],
      name: "two handle rails",
    },
  ] as const)(
    "uses the requested horizontal inset for $name",
    ({ expected, handles }) => {
      renderCompactShell(handles);

      const content = document.querySelector("[data-graph-node-content]");
      expect(content).toHaveAttribute(
        "data-graph-node-content-inset",
        "compact",
      );
      expect(content?.className).toContain("py-3");
      for (const className of expected) {
        expect(content?.className).toContain(className);
      }
    },
  );

  it("keeps the default inset available to generic graph consumers", () => {
    renderPackageComponent(
      <GraphNodeShell handles={[]}>
        <span>Default content</span>
      </GraphNodeShell>,
    );

    const content = document.querySelector("[data-graph-node-content]");
    expect(content).toHaveAttribute("data-graph-node-content-inset", "default");
    expect(content?.className).toContain("px-3");
  });
});
