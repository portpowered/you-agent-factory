import { useLayoutEffect, useState } from "react";

import {
  createDashboardRegressionFixture,
  DASHBOARD_REGRESSION_SESSION_IDS,
} from "../../../../components/dashboard/fixtures";
import type { DashboardRegressionCanonicalSessionID } from "../../../../components/dashboard/fixtures/dashboard-regression";
import { DashboardSessionStoreTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { useDashboardSession } from "../../../dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import { SubmitWorkWidget } from "../submit-work-widget";

let activeFixture = createDashboardRegressionFixture();

async function fixtureFetchResponse(
  input: RequestInfo | URL,
  init?: RequestInit,
) {
  const response = await activeFixture.fetch(input, init);
  return {
    body: await response.text(),
    headers: response.headers,
    status: response.status,
    statusText: response.statusText,
  };
}

function selectFixtureSession(
  sessionID: DashboardRegressionCanonicalSessionID,
) {
  activeFixture.selectSession(sessionID);
  useDashboardSessionStore.getState().setSelectedSessionID(sessionID);
}

function submitWorkRegressionParameters() {
  return {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: `/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.default}/work`,
          response: fixtureFetchResponse,
        },
        {
          method: "POST",
          path: `/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.secondary}/work`,
          response: fixtureFetchResponse,
        },
      ],
      sessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
    },
  };
}

export default {
  title: "you-agent-factory/Submit Work/Story 0 Regression Fixture",
  component: SubmitWorkWidget,
  tags: ["test"],
};

export const SessionScopedDelayedSubmission = {
  parameters: submitWorkRegressionParameters(),
  render: () => <SubmitWorkRegressionStory />,
};

function SubmitWorkRegressionStory() {
  const [isReady, setIsReady] = useState(false);

  useLayoutEffect(() => {
    activeFixture = createDashboardRegressionFixture();
    useDashboardSessionStore.setState({
      selectedSessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
    });
    setIsReady(true);
  }, []);

  if (!isReady) {
    return null;
  }

  return (
    <DashboardSessionStoreTestProvider>
      <div className="grid min-w-0 gap-4 p-4">
        <fieldset className="flex min-w-0 flex-wrap gap-2 border-0 p-0">
          <legend className="sr-only">Fixture Factory Session controls</legend>
          <button
            onClick={() =>
              selectFixtureSession(DASHBOARD_REGRESSION_SESSION_IDS.default)
            }
            type="button"
          >
            Select session A
          </button>
          <button
            onClick={() =>
              selectFixtureSession(DASHBOARD_REGRESSION_SESSION_IDS.secondary)
            }
            type="button"
          >
            Select session B
          </button>
          <button
            onClick={() =>
              activeFixture.submissions.resolve("sessionA", "success")
            }
            type="button"
          >
            Complete session A success
          </button>
          <button
            onClick={() =>
              activeFixture.submissions.resolve("sessionA", "failure")
            }
            type="button"
          >
            Complete session A failure
          </button>
          <button
            onClick={() =>
              activeFixture.submissions.resolve("sessionB", "success")
            }
            type="button"
          >
            Complete session B success
          </button>
          <button
            onClick={() =>
              activeFixture.submissions.resolve("sessionB", "failure")
            }
            type="button"
          >
            Complete session B failure
          </button>
        </fieldset>
        <FixtureSessionScope />
        <SubmitWorkWidget
          factoryState="RUNNING"
          submitWorkTypes={[{ work_type_name: "story" }]}
        />
      </div>
    </DashboardSessionStoreTestProvider>
  );
}

function FixtureSessionScope() {
  const { sessionID } = useDashboardSession();

  return (
    <output aria-label="Selected Factory Session" data-fixture-session-scope>
      {sessionID}
    </output>
  );
}
