// @vitest-environment node

import http from "node:http";

import { describe, expect, it } from "vitest";

import { previewHost, waitForPortAvailable } from "./browser-test-harness.mjs";

describe("browser port availability helpers", () => {
  it("waitForPortAvailable resolves when the port is already free", async () => {
    const server = http.createServer();
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, previewHost, resolve);
    });
    const address = server.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    await server.close();
    await waitForPortAvailable(previewHost, port, 1_000, 25);
  });

  it("waitForPortAvailable waits until a held port is released", async () => {
    const holder = http.createServer();
    await new Promise((resolve, reject) => {
      holder.once("error", reject);
      holder.listen(0, previewHost, resolve);
    });
    const address = holder.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    const releaseTimer = setTimeout(() => {
      holder.close();
    }, 100);

    await waitForPortAvailable(previewHost, port, 2_000, 25);
    clearTimeout(releaseTimer);
  });

  it("waitForPortAvailable rejects when the port stays busy", async () => {
    const holder = http.createServer();
    await new Promise((resolve, reject) => {
      holder.once("error", reject);
      holder.listen(0, previewHost, resolve);
    });
    const address = holder.address();
    const port = typeof address === "object" && address ? address.port : null;
    expect(port).toBeTypeOf("number");

    await expect(
      waitForPortAvailable(previewHost, port, 100, 25),
    ).rejects.toThrow(/Timed out waiting for/);

    await new Promise((resolve, reject) => {
      holder.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve();
      });
    });
  });
});
