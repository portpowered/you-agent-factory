import type { ReactNode } from "react";
import { vi } from "vitest";

/**
 * Stub WorkOutcomeWidget for App shell tests that do not assert on outcome charts.
 * Import as a side effect before app-shell-test-utils in those suites only.
 */
vi.mock("../features/work-outcome/hooks/useWorkOutcomeChart", () => ({
  useWorkOutcomeChart: () => ({ status: "empty" as const }),
}));

vi.mock("../features/work-outcome/components/work-outcome-widget", () => ({
  WorkOutcomeWidget: ({ headerAction }: { headerAction?: ReactNode }) => (
    <section data-testid="app-shell-work-outcome-stub">
      {headerAction}
      Work outcome card
    </section>
  ),
}));
