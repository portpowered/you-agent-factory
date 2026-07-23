import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";

import { FactoryInvocationExamples } from "./factory-invocation-examples";

it("renders exact localized example descriptions and base fallbacks", () => {
  render(
    <FactoryInvocationExamples
      examples={[
        {
          args: { input: "hello" },
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Exact base value.",
            values: { en: "Exact localized value." },
          },
          name: "Exact locale",
        },
        {
          args: {},
          description: {
            type: "LOCALIZABLE_ASSET",
            value: "Non-exact locale fallback.",
            values: { "en-US": "Region-specific value." },
          },
          name: "Fallback locale",
        },
      ]}
      locale="en"
      title="Examples"
    />,
  );

  expect(screen.getByText("Exact localized value.")).toBeVisible();
  expect(screen.queryByText("Exact base value.")).not.toBeInTheDocument();
  expect(screen.getByText("Non-exact locale fallback.")).toBeVisible();
  expect(screen.queryByText("Region-specific value.")).not.toBeInTheDocument();
});
