import "../../../../styles.css";

import { useLayoutEffect, useState } from "react";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { createDashboardRegressionFixture } from "../../../../components/dashboard/fixtures";
import {
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
} from "../../../../stories/dashboardStorySupport";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import { DashboardSessionTabs } from "../dashboard-session-tabs";

let activeFactoryJourneyFixture = createDashboardRegressionFixture();

async function factoryJourneyFixtureFetchResponse(
  input: RequestInfo | URL,
  init?: RequestInit,
) {
  const response = await activeFactoryJourneyFixture.fetch(input, init);
  return {
    body: await response.text(),
    headers: response.headers,
    status: response.status,
    statusText: response.statusText,
  };
}

async function resolveFactoryOperationAndRefresh(
  operationID: "open-confirm-success" | "new-confirm-success",
  requestID: "initial" | "refreshed",
) {
  activeFactoryJourneyFixture.sessionLists.enqueueFetch(requestID);
  activeFactoryJourneyFixture.factoryJourneys.resolve(operationID);
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (
      activeFactoryJourneyFixture
        .state()
        .pendingSessionListIDs.includes(requestID)
    ) {
      activeFactoryJourneyFixture.sessionLists.resolve(requestID);
      return;
    }
    await new Promise((resolve) => window.setTimeout(resolve, 10));
  }
  throw new Error(`Factory journey did not request the ${requestID} list.`);
}

export default {
  title: "you-agent-factory/Dashboard/Session Tabs/Open New Factory",
  component: DashboardSessionTabs,
  tags: ["test"],
};

export const Journey = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions",
          response: factoryJourneyFixtureFetchResponse,
        },
        {
          method: "POST",
          path: "/factory-sessions",
          response: factoryJourneyFixtureFetchResponse,
        },
      ],
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => <OpenNewFactoryJourneyStory />,
};

function OpenNewFactoryJourneyStory() {
  const [isReady, setIsReady] = useState(false);

  useLayoutEffect(() => {
    activeFactoryJourneyFixture = createDashboardRegressionFixture();
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    setIsReady(true);
  }, []);

  if (!isReady) {
    return null;
  }

  return (
    <div className="grid min-w-0 gap-3" style={{ maxWidth: "960px" }}>
      <FactoryJourneyFixtureControls />
      <DashboardSessionTabs locale="en" />
    </div>
  );
}

function FactoryJourneyFixtureControls() {
  return (
    <fieldset
      className="fixed inset-x-2 bottom-2 z-[100] flex min-w-0 flex-wrap gap-2 border-0 bg-surface-container-high p-2 shadow-lg"
      data-factory-journey-controls
    >
      <legend className="sr-only">Fixture Factory journey controls</legend>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.sessionLists.resolve("initial");
        }}
        type="button"
      >
        Resolve initial session list
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "open-validation-success",
          );
        }}
        type="button"
      >
        Resolve folder validation
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "open-validation-failure",
          );
        }}
        type="button"
      >
        Resolve Open validation failure
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "new-validation-success",
          );
        }}
        type="button"
      >
        Resolve New validation
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "new-validation-failure",
          );
        }}
        type="button"
      >
        Resolve New validation failure
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "new-validation-broken-success",
          );
        }}
        type="button"
      >
        Resolve New broken validation
      </button>
      <button
        onClick={() => {
          void resolveFactoryOperationAndRefresh(
            "open-confirm-success",
            "initial",
          );
        }}
        type="button"
      >
        Resolve Open success
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "open-confirm-failure",
          );
        }}
        type="button"
      >
        Resolve Open failure
      </button>
      <button
        onClick={() => {
          void resolveFactoryOperationAndRefresh(
            "new-confirm-success",
            "refreshed",
          );
        }}
        type="button"
      >
        Resolve New success
      </button>
      <button
        onClick={() => {
          activeFactoryJourneyFixture.factoryJourneys.resolve(
            "new-confirm-failure",
          );
        }}
        type="button"
      >
        Resolve New failure
      </button>
    </fieldset>
  );
}
