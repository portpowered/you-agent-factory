import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";

import * as factoryDocumentSaveHooks from "../features/current-factory-definition/hooks/useFactoryDocumentSave";
import { useEditableFactoryGraph } from "../features/factory-graph-editor/hooks/use-editable-factory-graph";
import type { UseEditableFactoryGraphOptions } from "../features/factory-graph-editor/hooks/use-editable-factory-graph-types";
import { DashboardSessionStoreTestProvider } from "./dashboard-session-test-provider";
import {
  type MockFactoryDocumentSaveReturn,
  mockFactoryDocumentSave,
} from "./factory-document-save-mocks";
import { seedScopedFactoryDocumentSaveTestSession } from "./scoped-factory-document-save-test-helpers";

export const defaultGraphDocumentScopeKey = "session-default";

export function setupEditableFactoryGraphSaveTestEnvironment(
  saveMutation: MockFactoryDocumentSaveReturn = mockFactoryDocumentSave({
    mode: "success",
  }),
) {
  seedScopedFactoryDocumentSaveTestSession();
  vi.restoreAllMocks();
  vi.spyOn(factoryDocumentSaveHooks, "useFactoryDocumentSave").mockReturnValue(
    saveMutation as never,
  );
  return saveMutation;
}

export function createEditableFactoryGraphHookWrapper(
  queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  }),
) {
  function EditableFactoryGraphHookWrapper({
    children,
  }: {
    children: ReactNode;
  }) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
      </QueryClientProvider>
    );
  }

  return { EditableFactoryGraphHookWrapper, queryClient };
}

export function renderEditableFactoryGraphHook(
  options: UseEditableFactoryGraphOptions = {},
  queryClient?: QueryClient,
) {
  const { EditableFactoryGraphHookWrapper } =
    createEditableFactoryGraphHookWrapper(queryClient);

  return renderHook(
    () =>
      useEditableFactoryGraph({
        factoryDocumentScopeKey: defaultGraphDocumentScopeKey,
        ...options,
      }),
    { wrapper: EditableFactoryGraphHookWrapper },
  );
}
