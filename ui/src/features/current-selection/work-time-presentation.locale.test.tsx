import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { dashboardWorkstationRequestFixtures } from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { formatLocalDateTime } from "../../components/ui/formatters";
import { formatTime } from "../../i18n/formatters";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
} from "./base/components/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "./base/components/current-selection-locale";
import { StateNodeDetailCard } from "./work-state-selection/components/state-node-detail";
import { selectWorkItemExecutionDetails } from "./work-selection/state/executionDetails";
import { WorkItemDetailCard } from "./work-selection/components/work-item-card";

const sharedStartedAt = "2026-04-08T12:00:01Z";

const localeCases = [
  {
    locale: "en" as const,
    unavailableLabel: "Unavailable",
    startedAtLabel: "Started at",
    dispatchHistoryRegion: "Workstation dispatches",
  },
  {
    locale: "zh-CN" as const,
    unavailableLabel: "不可用",
    startedAtLabel: "开始时间",
    dispatchHistoryRegion: "工作站分派",
  },
] as const;

function getDetailRow(container: HTMLElement, label: string): HTMLElement {
  const term = within(container).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected detail row for ${label}`);
  }

  return row;
}

function requireValue<T>(value: T | null | undefined, message: string): T {
  if (value === null || value === undefined) {
    throw new Error(message);
  }

  return value;
}

function renderWorkStateStartedAt(locale: (typeof localeCases)[number]["locale"]): string {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const selectedState = snapshot.topology.workstation_nodes_by_id.review.input_places?.find(
    (place) => place.place_id === "story:implemented",
  );

  render(
    <CurrentSelectionLocaleProvider locale={locale}>
      <StateNodeDetailCard
        currentWorkItems={[
          {
            display_name: "Active Story",
            started_at: sharedStartedAt,
            work_id: "work-active-story",
          },
        ]}
        place={requireValue(selectedState, "expected implemented state fixture")}
        tokenCount={1}
      />
    </CurrentSelectionLocaleProvider>,
  );

  const unavailableLabel = locale === "zh-CN" ? "不可用" : "Unavailable";
  const expected = formatLocalDateTime(sharedStartedAt, unavailableLabel, locale);
  const startedAtPrefix = locale === "zh-CN" ? "开始时间 " : "Started at ";

  expect(screen.getByText(`${startedAtPrefix}${expected}`)).toBeTruthy();

  return expected;
}

function renderDispatchStartedAt(
  locale: (typeof localeCases)[number]["locale"],
  startedAtLabel: string,
  dispatchHistoryRegion: string,
): string {
  const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();
  const unavailableLabel = locale === "zh-CN" ? "不可用" : "Unavailable";
  const expected = formatLocalDateTime(sharedStartedAt, unavailableLabel, locale);

  render(
    <CurrentSelectionLocaleProvider locale={locale}>
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[dashboardWorkstationRequestFixtures.ready]}
      />
    </CurrentSelectionLocaleProvider>,
  );

  const dispatchCard = within(screen.getByRole("region", { name: dispatchHistoryRegion }))
    .getByText("Active Story", { selector: "strong" })
    .closest("article");

  if (!(dispatchCard instanceof HTMLElement)) {
    throw new Error("expected dispatch history card");
  }

  expect(
    within(getDetailRow(dispatchCard, startedAtLabel)).getByText(expected),
  ).toBeTruthy();

  return expected;
}

describe("work time presentation locale regression", () => {
  it.each(localeCases)(
    "renders the same canonical Started at output on work-state and dispatch surfaces for $locale",
    ({ locale, unavailableLabel, startedAtLabel, dispatchHistoryRegion }) => {
      const expected = formatLocalDateTime(
        sharedStartedAt,
        unavailableLabel,
        locale,
      );
      const hourOnly = formatTime(sharedStartedAt, locale);

      expect(expected).not.toBe(hourOnly);

      const workStateStartedAt = renderWorkStateStartedAt(locale);
      const dispatchStartedAt = renderDispatchStartedAt(
        locale,
        startedAtLabel,
        dispatchHistoryRegion,
      );

      expect(workStateStartedAt).toBe(expected);
      expect(dispatchStartedAt).toBe(expected);
      expect(workStateStartedAt).toBe(dispatchStartedAt);
      expect(workStateStartedAt).not.toBe(hourOnly);
    },
  );
});
