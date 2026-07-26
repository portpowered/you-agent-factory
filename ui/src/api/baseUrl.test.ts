import { factoryAPIURL } from "./baseUrl";

describe("factoryAPIURL", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    delete (
      globalThis as typeof globalThis & {
        __agentFactoryBrowserTestAPIOrigin?: string;
      }
    ).__agentFactoryBrowserTestAPIOrigin;
  });

  it("uses a browser-test runtime origin without rebuilding immutable assets", () => {
    (
      globalThis as typeof globalThis & {
        __agentFactoryBrowserTestAPIOrigin?: string;
      }
    ).__agentFactoryBrowserTestAPIOrigin = "http://127.0.0.1:8123/";

    expect(factoryAPIURL("/events")).toBe("http://127.0.0.1:8123/events");
  });

  it("uses same-origin paths by default", () => {
    expect(factoryAPIURL("/events")).toBe("/events");
  });

  it("points API calls at the configured factory server", () => {
    vi.stubEnv("VITE_AGENT_FACTORY_API_ORIGIN", "http://127.0.0.1:7437/");

    expect(factoryAPIURL("/events")).toBe("http://127.0.0.1:7437/events");
    expect(factoryAPIURL("events")).toBe("http://127.0.0.1:7437/events");
  });
});
