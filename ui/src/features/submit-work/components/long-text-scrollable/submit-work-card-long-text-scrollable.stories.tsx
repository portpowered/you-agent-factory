import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { expect, within } from "storybook/test";

import { DashboardSessionTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { SubmitWorkCardLongTextScrollableVerification } from "./submit-work-card-long-text-scrollable-verification";

const withQueryClient = (Story: () => ReactElement) => (
  <QueryClientProvider
    client={
      new QueryClient({
        defaultOptions: {
          mutations: {
            retry: false,
          },
          queries: {
            retry: false,
          },
        },
      })
    }
  >
    <DashboardSessionTestProvider>
      <Story />
    </DashboardSessionTestProvider>
  </QueryClientProvider>
);

export default {
  title: "Agent Factory/Dashboard/Submit Work Card",
  decorators: [withQueryClient],
};

export const LongTextScrollableVerification = {
  tags: ["test"],
  render: () => <SubmitWorkCardLongTextScrollableVerification />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });
    const scope = within(card);
    const submissionTextarea = scope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    await expect(submissionTextarea.className).toContain("max-h-52");
    await expect(submissionTextarea.className).toContain("overflow-y-auto");
    await expect(submissionTextarea.className).toContain("af-styled-scrollbar");
    await expect(submissionTextarea.scrollHeight).toBeGreaterThan(
      submissionTextarea.clientHeight,
    );
    await expect(
      scope.getByRole("button", { name: "Submit work" }),
    ).toBeVisible();
  },
};
