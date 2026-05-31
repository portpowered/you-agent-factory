import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";

import * as factoryDocumentSaveHooks from "../features/current-factory-definition/hooks/useFactoryDocumentSave";
import type { UseEditableFactoryGraphOptions } from "../features/factory-graph-editor/hooks/use-editable-factory-graph-types";
import { useEditableFactoryGraph } from "../features/factory-graph-editor/hooks/use-editable-factory-graph";
import { DashboardSessionProvider } from "../features/dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";
import {
  mockFactoryDocumentSave,
  type MockFactoryDocumentSaveReturn,
} from "./factory-document-save-mocks";

export const defaultGraphDocumentScopeKey = "session-default";

export function setupEditableFactoryGraphSaveTestEnvironment(
  saveMutation: MockFactoryDocumentSaveReturn = mockFactoryDocumentSave({
    mode: "success",
  }),
) {
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
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
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
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
