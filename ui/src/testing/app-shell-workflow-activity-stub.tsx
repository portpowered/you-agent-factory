import type { ReactNode } from "react";
import { vi } from "vitest";

/**
 * Stub WorkflowActivityWidget for App shell tests that do not assert on the factory graph.
 * Import as a side effect before app-shell-test-utils in those suites only.
 */
vi.mock("../features/workflow-activity/components/workflow-activity-widget", () => ({
  WorkflowActivityWidget: ({
    headerAction,
    widgetInstanceID,
  }: {
    headerAction?: ReactNode;
    widgetInstanceID?: string;
  }) => (
    <section data-testid="app-shell-workflow-activity-stub">
      {headerAction}
      Workflow activity card
      {widgetInstanceID ? `:${widgetInstanceID}` : ""}
    </section>
  ),
}));
