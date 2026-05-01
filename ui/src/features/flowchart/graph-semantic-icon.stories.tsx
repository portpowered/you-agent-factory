import { expect, within } from "storybook/test";

import "../../styles.css";
import {
  GRAPH_SEMANTIC_ICON_KINDS,
  GraphSemanticIcon,
  graphSemanticIconLabel,
} from "./graph-semantic-icon";

export default {
  title: "Agent Factory/Dashboard/Graph Semantic Icon",
  component: GraphSemanticIcon,
  tags: ["test"],
};

export const Vocabulary = {
  render: () => (
    <div className="grid max-w-3xl grid-cols-[repeat(auto-fit,minmax(8rem,1fr))] gap-3 bg-af-bg p-4 text-af-ink">
      {GRAPH_SEMANTIC_ICON_KINDS.map((kind) => (
        <div
          className="flex items-center gap-2 rounded-lg border border-af-overlay/10 bg-af-surface/88 p-3"
          key={kind}
        >
          <GraphSemanticIcon className="h-5 w-5 text-af-info" kind={kind} />
          <span className="text-sm font-bold">{graphSemanticIconLabel(kind)}</span>
        </div>
      ))}
      <div className="flex items-center gap-2 rounded-lg border border-af-overlay/10 bg-af-surface/88 p-3">
        <GraphSemanticIcon className="h-5 w-5 text-af-accent" kind="future-node-kind" />
        <span className="text-sm font-bold">Fallback</span>
      </div>
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    for (const kind of GRAPH_SEMANTIC_ICON_KINDS) {
      await expect(canvas.getByRole("img", { name: graphSemanticIconLabel(kind) })).toBeVisible();
    }
    await expect(canvas.getByRole("img", { name: "Unknown graph semantic" })).toBeVisible();
  },
};
