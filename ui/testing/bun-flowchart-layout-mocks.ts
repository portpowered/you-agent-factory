/**
 * Partial mock for `flowchart/lib/layout` with optional buildGraphLayout override.
 */
import { mock } from "bun:test";

const FLOWCHART_LAYOUT_MODULE = "../src/features/flowchart/lib/layout";

const flowchartLayoutActual = await import(FLOWCHART_LAYOUT_MODULE);

export const mockBuildGraphLayout = mock(
  (...args: Parameters<typeof flowchartLayoutActual.buildGraphLayout>) =>
    flowchartLayoutActual.buildGraphLayout(...args),
);

mock.module(FLOWCHART_LAYOUT_MODULE, () => ({
  ...flowchartLayoutActual,
  buildGraphLayout: (
    ...args: Parameters<typeof flowchartLayoutActual.buildGraphLayout>
  ) => {
    const implementation = mockBuildGraphLayout.getMockImplementation();
    if (implementation) {
      return mockBuildGraphLayout(...args);
    }

    return flowchartLayoutActual.buildGraphLayout(...args);
  },
}));
