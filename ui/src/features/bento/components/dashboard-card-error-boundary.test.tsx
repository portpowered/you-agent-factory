import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  AgentBentoCard,
  AgentBentoLayout,
  type AgentBentoLayoutCard,
} from "./agent-bento";

const layout = [
  { h: 2, id: "broken", widgetType: "broken", w: 6, x: 0, y: 0 },
  { h: 2, id: "healthy", widgetType: "healthy", w: 6, x: 6, y: 0 },
] as const;

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
  Object.defineProperty(HTMLElement.prototype, "offsetParent", {
    configurable: true,
    get() {
      return this.parentElement ?? document.body;
    },
  });
});

afterEach(() => {
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
  vi.restoreAllMocks();
});

function renderBoard(cards: AgentBentoLayoutCard[]) {
  return render(
    <AgentBentoLayout cards={cards} initialWidth={960} layout={[...layout]} />,
  );
}

function HealthyCard() {
  const [clicks, setClicks] = useState(0);

  return (
    <AgentBentoCard title="Healthy card">
      <button onClick={() => setClicks((current) => current + 1)} type="button">
        Sibling action {clicks}
      </button>
    </AgentBentoCard>
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the two cases share one board fixture and cover the complete localized recovery contract.
describe("DashboardCardErrorBoundary", () => {
  it("contains one failed card, preserves sibling state and geometry, and recovers only that card", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    let shouldThrow = true;

    function RecoveringCard() {
      if (shouldThrow) {
        throw new Error("private render details must stay hidden");
      }

      return (
        <AgentBentoCard title="Recovering card">
          <p>Recovered card content</p>
        </AgentBentoCard>
      );
    }

    try {
      renderBoard([
        {
          id: "broken",
          widgetType: "broken",
          children: <RecoveringCard />,
        },
        {
          id: "healthy",
          widgetType: "healthy",
          children: <HealthyCard />,
        },
      ]);

      const board = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });
      const brokenGridItem = board.querySelector<HTMLElement>(
        '[data-bento-instance-id="broken"]',
      );
      if (!brokenGridItem) {
        throw new Error("expected the failed card grid item");
      }

      const layoutSignature = brokenGridItem.dataset.layoutSignature;
      expect(
        within(
          screen.getByRole("article", { name: "Dashboard card unavailable" }),
        ).getByRole("alert").textContent,
      ).toContain("could not be rendered");
      expect(
        screen.queryByText("private render details must stay hidden"),
      ).toBeNull();
      expect(
        screen.getByRole("article", { name: "Healthy card" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("button", { name: "Move Healthy card card" }),
      ).toBeTruthy();

      const siblingAction = screen.getByRole("button", {
        name: "Sibling action 0",
      });
      fireEvent.click(siblingAction);
      expect(
        screen.getByRole("button", { name: "Sibling action 1" }),
      ).toBeTruthy();

      shouldThrow = false;
      const retryButton = within(
        screen.getByRole("article", { name: "Dashboard card unavailable" }),
      ).getByRole("button", { name: "Retry card" });
      retryButton.focus();
      expect(document.activeElement).toBe(retryButton);
      expect(retryButton.className).toContain("focus-visible:ring");
      await userEvent.setup().keyboard("{Enter}");

      await waitFor(() => {
        expect(
          screen.getByRole("article", { name: "Recovering card" }),
        ).toBeTruthy();
      });

      expect(
        screen.getByRole("button", { name: "Sibling action 1" }),
      ).toBeTruthy();
      expect(brokenGridItem.dataset.layoutSignature).toBe(layoutSignature);
      expect(board.querySelector('[data-bento-instance-id="broken"]')).toBe(
        brokenGridItem,
      );
      expect(consoleError).toHaveBeenCalled();
    } finally {
      consoleError.mockRestore();
    }
  });

  it("keeps repeated render failures localized with retry available", () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);

    function AlwaysFailingCard() {
      throw new Error("repeated private render details");
    }

    try {
      renderBoard([
        {
          id: "broken",
          widgetType: "broken",
          children: <AlwaysFailingCard />,
        },
        {
          id: "healthy",
          widgetType: "healthy",
          children: <HealthyCard />,
        },
      ]);

      const failedCard = screen.getByRole("article", {
        name: "Dashboard card unavailable",
      });
      fireEvent.click(
        within(failedCard).getByRole("button", { name: "Retry card" }),
      );

      expect(
        screen.getByRole("article", { name: "Dashboard card unavailable" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("article", { name: "Healthy card" }),
      ).toBeTruthy();
      expect(screen.queryByText("repeated private render details")).toBeNull();
      expect(
        within(
          screen.getByRole("article", { name: "Dashboard card unavailable" }),
        ).getByRole("button", { name: "Retry card" }),
      ).toBeTruthy();
    } finally {
      consoleError.mockRestore();
    }
  });
});
