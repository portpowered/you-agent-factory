import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PackagedFactoryCatalogResponse } from "../../../api/packaged-factories";
import { PackagedFactoryInventory } from "./packaged-factory-inventory";

function catalogEntry(
  slug: string,
  description: string,
  localized?: string,
  overrides: Partial<PackagedFactoryCatalogResponse["factories"][number]> = {},
) {
  return {
    description: {
      type: "LOCALIZABLE_ASSET" as const,
      value: description,
      values: localized ? { "zh-CN": localized } : undefined,
    },
    examples: [
      {
        name: "default",
        description: {
          type: "LOCALIZABLE_ASSET" as const,
          value: `Run ${slug}`,
        },
        args: { input: `Run ${slug}` },
      },
    ],
    json: { id: `builtin-${slug}`, name: slug },
    name: `@you/${slug}`,
    project: `builtin-${slug}`,
    slug,
    yaml: `id: builtin-${slug}\nname: ${slug}\n`,
    ...overrides,
  } satisfies PackagedFactoryCatalogResponse["factories"][number];
}

function catalog(
  factories = [
    catalogEntry("alpha", "Alpha description", "Alpha 中文说明"),
    catalogEntry("beta", "Beta description"),
  ],
): PackagedFactoryCatalogResponse {
  return { factories };
}

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}

function renderInventory() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <PackagedFactoryInventory />
    </QueryClientProvider>,
  );
}

describe("PackagedFactoryInventory API states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows loading and empty states from the backend response", async () => {
    vi.mocked(globalThis.fetch).mockReturnValue(new Promise(() => {}));
    const { unmount } = renderInventory();
    expect(screen.getByText("Loading Packaged Factories…")).toBeVisible();
    unmount();

    vi.mocked(globalThis.fetch).mockResolvedValueOnce(response(catalog([])));
    renderInventory();
    expect(
      await screen.findByText("No Packaged Factories are available."),
    ).toBeVisible();
  });

  it("shows a recoverable API error and retries through the typed boundary", async () => {
    const user = userEvent.setup();
    vi.mocked(globalThis.fetch)
      .mockResolvedValueOnce(response({ code: "INTERNAL_ERROR" }, 500))
      .mockResolvedValueOnce(response(catalog()));
    renderInventory();

    expect(
      await screen.findByText("The Packaged Factory catalog is unavailable."),
    ).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("heading", { level: 3, name: "@you/alpha" }),
    ).toBeVisible();
    expect(globalThis.fetch).toHaveBeenCalledTimes(2);
  });
});

describe("PackagedFactoryInventory selection", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders API-shaped catalog data and supports arrow traversal and keyboard activation", async () => {
    const user = userEvent.setup();
    vi.mocked(globalThis.fetch).mockResolvedValueOnce(response(catalog()));
    const { baseElement } = renderInventory();

    const alpha = await screen.findByRole("button", { name: /@you\/alpha/ });
    const beta = screen.getByRole("button", {
      name: /@you\/beta/,
    });
    expect(screen.getAllByText("@you/alpha")).toHaveLength(2);
    expect(alpha.getAttribute("aria-current")).toBe("true");
    expect(alpha.getAttribute("aria-pressed")).toBe("true");

    alpha.focus();
    fireEvent.keyDown(alpha, { key: "ArrowDown" });
    expect(document.activeElement).toBe(beta);
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(beta.getAttribute("aria-current")).toBe("true");
    });
    expect(
      await screen.findByRole("heading", {
        level: 3,
        name: "@you/beta",
      }),
    ).toBeVisible();
    expect(
      screen.getByText("@you/beta selected").getAttribute("aria-live"),
    ).toBe("polite");
    expect((await axe(baseElement)).violations).toEqual([]);
  });

  it("does not render stale detail after a selected artifact failure and recovers through another selection", async () => {
    const user = userEvent.setup();
    vi.mocked(globalThis.fetch).mockResolvedValueOnce(
      response(
        catalog([
          catalogEntry("alpha", "Alpha description", undefined, {
            yaml: "",
          }),
          catalogEntry("beta", "Beta description"),
        ]),
      ),
    );
    renderInventory();

    expect((await screen.findByRole("alert")).textContent).toContain(
      "This Factory could not be loaded. Select another Factory to continue.",
    );
    expect(
      screen.queryByRole("heading", { level: 3, name: "@you/alpha" }),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: /@you\/beta/ }));

    expect(
      await screen.findByRole("heading", { level: 3, name: "@you/beta" }),
    ).toBeVisible();
    expect(
      screen.queryByText(
        "This Factory could not be loaded. Select another Factory to continue.",
      ),
    ).toBeNull();
  });

  it("uses localized copy and renders API artifact content as inert escaped text", async () => {
    const hostile = "<img src=x onerror=alert(1)>";
    vi.mocked(globalThis.fetch).mockResolvedValueOnce(
      response(
        catalog([
          catalogEntry("alpha", hostile, hostile, {
            examples: [
              {
                name: hostile,
                description: {
                  type: "LOCALIZABLE_ASSET",
                  value: hostile,
                },
                args: { input: hostile },
              },
            ],
          }),
        ]),
      ),
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <PackagedFactoryInventory locale="zh-CN" />
      </QueryClientProvider>,
    );

    expect(
      await screen.findByRole("navigation", { name: "可用的打包工厂" }),
    ).toBeVisible();
    expect(screen.getAllByText(hostile).length).toBeGreaterThan(0);
    expect(document.querySelector("img")).toBeNull();
  });
});
