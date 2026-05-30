/**
 * App-shell layout and factory-definition module mocks for Bun unit tests.
 * Import this module before app-shell-test-utils so mock.module runs before
 * consumers load the real modules (Bun does not hoist mocks like Vitest).
 */
import { mock } from "bun:test";

/** Resolved the same way as `../features/...` imports from `ui/src/testing/`. */
const LAYOUT_MODULE = "../src/features/flowchart/lib/layout";
const FACTORY_DEFINITION_MODULE =
  "../src/features/current-factory-definition/public";

export const useCurrentFactoryDocumentMock = mock(() => {
  throw new Error("useCurrentFactoryDocumentMock not configured");
});

(
  globalThis as typeof globalThis & {
    __useCurrentFactoryDocumentMock?: typeof useCurrentFactoryDocumentMock;
  }
).__useCurrentFactoryDocumentMock = useCurrentFactoryDocumentMock;

const layoutActual = await import(LAYOUT_MODULE);
const { buildDashboardTestGraphLayout } = await import(
  "../src/testing/app-shell-test-graph-layout"
);

mock.module(LAYOUT_MODULE, () => ({
  ...layoutActual,
  buildGraphLayout: async (
    topology: Parameters<typeof buildDashboardTestGraphLayout>[0],
  ) => buildDashboardTestGraphLayout(topology),
}));

const factoryDefinitionActual = await import(FACTORY_DEFINITION_MODULE);

mock.module(FACTORY_DEFINITION_MODULE, () => ({
  ...factoryDefinitionActual,
  useCurrentFactoryDocument: useCurrentFactoryDocumentMock,
}));
