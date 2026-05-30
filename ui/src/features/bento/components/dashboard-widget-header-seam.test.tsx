import { render, screen, within } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it } from "vitest";

import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { ProviderSessionWidget } from "../../provider-session-detail/components/provider-session-widget";
import { getProviderSessionWidgetMessages } from "../../provider-session-detail/messages/provider-session-widget";
import { SubmitWorkCard } from "../../submit-work/components/submit-work-card";
import { getSubmitWorkMessages } from "../../submit-work/messages/submit-work";
import { getTerminalWorkMessages } from "../../terminal-work/messages/terminal-work";
import { TerminalWorkWidget } from "../../terminal-work/public";
import { TraceDrilldownWidget } from "../../trace-drilldown/components/trace-drilldown-widget";
import { getTraceDrilldownMessages } from "../../trace-drilldown/messages/trace-drilldown";
import { D3CompletionInformationCard } from "../../work-outcome/components/d3-information-card";
import type { WorkChartModel } from "../../work-outcome/lib/trends";

const minimalWorkChartModel: WorkChartModel = {
  delta: {
    queued: 0,
    inFlight: 0,
    completed: 0,
    failed: 0,
  },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

function expectSharedBentoCardHeaderSeam(
  card: HTMLElement,
  {
    title,
    headerActionLabel,
  }: {
    title: string;
    headerActionLabel?: string;
  },
) {
  const cardHeader = card.querySelector("header");
  expect(cardHeader).toBeTruthy();

  const titleHeading = within(cardHeader as HTMLElement).getByRole("heading", {
    level: 3,
    name: title,
  });
  expect(titleHeading.className).toContain(DASHBOARD_SECTION_HEADING_CLASS);

  expect(cardHeader?.getAttribute("data-bento-drag-handle")).toBe("true");
  expect(cardHeader?.className).toContain("cursor-grab");
  expect(
    within(card).queryByRole("button", { name: `Move ${title}` }),
  ).toBeNull();

  if (headerActionLabel) {
    const headerTools = cardHeader?.querySelector(
      "[class*='shrink-0'][class*='items-center']",
    );
    const headerAction = within(card).getByRole("button", {
      name: headerActionLabel,
    });
    expect(headerTools).toBeTruthy();
    expect(headerTools?.contains(headerAction)).toBe(true);
    expect(titleHeading.compareDocumentPosition(headerAction)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  }
}

function renderBentoWidget(ui: ReactElement) {
  const view = render(ui);
  return view;
}

describe("dashboard widget header seam", () => {
  it("routes provider session through the shared bento card header", () => {
    const messages = getProviderSessionWidgetMessages();
    renderBentoWidget(
      <ProviderSessionWidget
        headerAction={null}
        selectedProviderSession={null}
      />,
    );

    const card = screen.getByRole("article", { name: messages.title });
    expectSharedBentoCardHeaderSeam(card, { title: messages.title });
    expect(screen.getByText(messages.emptyState)).toBeTruthy();
  });

  it("routes trace drilldown through the shared bento card header without a second card title row", () => {
    const messages = getTraceDrilldownMessages();
    renderBentoWidget(
      <TraceDrilldownWidget
        headerAction={<button type="button">Remove trace card</button>}
        state={{
          message: messages.idleMessage,
          status: "idle",
        }}
      />,
    );

    const card = screen.getByRole("article", { name: messages.title });
    expectSharedBentoCardHeaderSeam(card, {
      headerActionLabel: "Remove trace card",
      title: messages.title,
    });
    expect(
      within(card).getByRole("heading", { level: 3, name: messages.idleTitle }),
    ).toBeTruthy();
    expect(
      within(card.querySelector("header") as HTMLElement).queryByRole(
        "heading",
        {
          name: messages.idleTitle,
        },
      ),
    ).toBeNull();
  });

  it("routes work outcome chart through the shared bento card header", () => {
    renderBentoWidget(
      <D3CompletionInformationCard
        headerAction={<button type="button">Remove chart</button>}
        model={minimalWorkChartModel}
      />,
    );

    const card = screen.getByRole("article", { name: "Work outcome chart" });
    expectSharedBentoCardHeaderSeam(card, {
      headerActionLabel: "Remove chart",
      title: "Work outcome chart",
    });
  });

  it("routes terminal work through the shared bento card header", () => {
    const messages = getTerminalWorkMessages();
    renderBentoWidget(
      <TerminalWorkWidget
        completedItems={[]}
        failedItems={[]}
        headerAction={<button type="button">Remove terminal work</button>}
        onSelectItem={() => {}}
        selectedItem={null}
      />,
    );

    const card = screen.getByRole("article", { name: messages.cardTitle });
    expectSharedBentoCardHeaderSeam(card, {
      headerActionLabel: "Remove terminal work",
      title: messages.cardTitle,
    });
    expect(
      within(card).getByRole("heading", {
        name: messages.rowTitle("completed"),
      }),
    ).toBeTruthy();
    expect(
      within(card.querySelector("header") as HTMLElement).queryByRole(
        "heading",
        {
          name: messages.rowTitle("completed"),
        },
      ),
    ).toBeNull();
  });

  it("routes submit work header controls through the shared tools region without a separate move handle", () => {
    const messages = getSubmitWorkMessages();
    renderBentoWidget(
      <SubmitWorkCard
        draft={{
          items: [{ id: "submission-item-1", text: "", type: "text" }],
          requestName: "",
          workTypeName: "",
        }}
        headerAction={
          <button type="button">
            Remove submit work widget from dashboard
          </button>
        }
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{
          kind: "guidance",
          message: messages.statusMessages.emptyGuidance,
        }}
        submitWorkTypeNames={["story"]}
      />,
    );

    const card = screen.getByRole("article", { name: messages.cardTitle });
    expectSharedBentoCardHeaderSeam(card, {
      headerActionLabel: "Remove submit work widget from dashboard",
      title: messages.cardTitle,
    });
    expect(
      within(card).getByRole("combobox", { name: messages.workTypeLabel }),
    ).toBeTruthy();
  });
});
