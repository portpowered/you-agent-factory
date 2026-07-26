// Dashboard composition belongs to the feature; generic test helpers stay screen-agnostic.
import {
  type RenderAppOptions,
  type RenderAppResult,
  renderApp,
  renderAppWithDashboardShell,
} from "../../../../testing/app-shell-test-utils";
import { DashboardScreen } from "../dashboard-screen";

export type DashboardScreenTestRenderOptions = Omit<RenderAppOptions, "app">;

export function renderDashboardScreen(
  options: DashboardScreenTestRenderOptions,
): RenderAppResult {
  return renderApp({ ...options, app: <DashboardScreen /> });
}

export function renderDashboardScreenWithShell(
  options: DashboardScreenTestRenderOptions,
): Promise<RenderAppResult> {
  return renderAppWithDashboardShell({
    ...options,
    app: <DashboardScreen />,
  });
}
