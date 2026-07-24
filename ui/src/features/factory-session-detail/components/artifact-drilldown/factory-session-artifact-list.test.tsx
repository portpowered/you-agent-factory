import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";

import { FactorySessionArtifactList } from "./factory-session-artifact-list";

describe("FactorySessionArtifactList", () => {
  it("keeps the drilldown body empty until a session context exists", async () => {
    const user = userEvent.setup();

    renderWithQueryClient(
      <FactorySessionArtifactList
        artifacts={[
          {
            id: "artifact-idle",
            kind: "FINAL_RESULT",
            visibility: "CUSTOMER",
          },
        ]}
        heading="Artifacts"
        sessionID={null}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "View artifact artifact-idle" }),
    );

    expect(screen.queryByText("Artifact detail")).toBeNull();
    expect(screen.queryByText("Loading artifact detail…")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

function renderWithQueryClient(node: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>,
  );
}
