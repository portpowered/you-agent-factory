import { fireEvent, render, screen, within } from "@testing-library/react";
import {
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

it("renders markdown-authored request and response bodies inside inference attempts", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-markdown", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-markdown", {
            attempt: 1,
            inference_request_id: "dispatch-review-markdown/inference-request/1",
            prompt: [
              "## Review checklist",
              "",
              "- Check the latest diff",
              "- Run `bun test` before approval",
              "",
              "```text",
              "bun test",
              "```",
            ].join("\n"),
            response: [
              "### Reviewer response",
              "",
              "1. Run `bun run lint`",
              "2. Confirm the diff is limited",
              "",
              "```text",
              "bun run lint",
              "```",
            ].join("\n"),
          }),
        ],
        request_id: "request-markdown-story",
      })}
    />,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand request body" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand response body" }),
  );

  const requestBody = within(
    inferenceAttempts.getByRole("region", { name: "Request body" }),
  );
  const responseBody = within(
    inferenceAttempts.getByRole("region", { name: "Response body" }),
  );
  const requestListItems = requestBody.getAllByRole("listitem");
  const responseListItems = responseBody.getAllByRole("listitem");

  expect(
    requestBody.getByRole("heading", { level: 2, name: "Review checklist" }),
  ).toBeTruthy();
  expect(requestBody.getByRole("list")).toBeTruthy();
  expect(requestBody.getByText("Check the latest diff")).toBeTruthy();
  expect(requestListItems[1]?.textContent).toBe("Run bun test before approval");
  expect(requestBody.getAllByText(/bun test/)).toHaveLength(2);
  expect(
    responseBody.getByRole("heading", {
      level: 3,
      name: "Reviewer response",
    }),
  ).toBeTruthy();
  expect(responseBody.getByRole("list")).toBeTruthy();
  expect(responseListItems[0]?.textContent).toBe("Run bun run lint");
  expect(responseBody.getByText("Confirm the diff is limited")).toBeTruthy();
  expect(responseBody.getAllByText(/bun run lint/)).toHaveLength(2);
});

it("renders ordered-list prompt bodies as structured lists inside inference attempts", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-ordered", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-ordered", {
            attempt: 1,
            inference_request_id: "dispatch-review-ordered/inference-request/1",
            prompt: ["1. Run `bun run lint`", "2. `bun run test:unit`"].join(
              "\n",
            ),
          }),
        ],
        request_id: "request-ordered-story",
      })}
    />,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand request body" }),
  );

  const requestBody = within(
    inferenceAttempts.getByRole("region", { name: "Request body" }),
  );
  const requestListItems = requestBody.getAllByRole("listitem");

  expect(requestBody.getByRole("list")).toBeTruthy();
  expect(requestListItems[0]?.textContent).toBe("Run bun run lint");
  expect(requestListItems[1]?.textContent).toBe("bun run test:unit");
});

it("renders plain-text prompts as readable request bodies inside inference attempts", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-plain-text", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-plain-text", {
            attempt: 1,
            inference_request_id:
              "dispatch-review-plain-text/inference-request/1",
            prompt: [
              "Review the current story before approval.",
              "Keep the existing response rendering unchanged.",
            ].join("\n"),
          }),
        ],
        request_id: "request-plain-text-story",
      })}
    />,
  );

  const inferenceAttemptsRegion = screen.getByRole("region", {
    name: "Inference attempts",
  });
  const inferenceAttempts = within(inferenceAttemptsRegion);
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand request body" }),
  );

  const requestBody = within(
    inferenceAttempts.getByRole("region", { name: "Request body" }),
  );

  expect(requestBody.queryByRole("heading", { level: 1 })).toBeNull();
  expect(requestBody.queryByRole("heading", { level: 2 })).toBeNull();
  expect(requestBody.queryByRole("heading", { level: 3 })).toBeNull();
  expect(requestBody.queryByRole("list")).toBeNull();
  expect(
    requestBody
      .getByText(/Review the current story before approval\./)
      .closest("p"),
  ).not.toBeNull();
  expect(inferenceAttemptsRegion.querySelectorAll("pre")).toHaveLength(0);
  expect(
    inferenceAttempts.getByText(/Review the current story before approval\./),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByText(
      /Keep the existing response rendering unchanged\./,
    ),
  ).toBeTruthy();
});

it("renders embedded raw html in prompts as inert text inside inference attempts", () => {
  const { container } = render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-html", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-html", {
            attempt: 1,
            inference_request_id: "dispatch-review-html/inference-request/1",
            prompt: '<button>danger</button>\n\n<script>alert("xss")</script>',
          }),
        ],
        request_id: "request-html-story",
      })}
    />,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand request body" }),
  );

  expect(
    inferenceAttempts.queryByRole("button", { name: "danger" }),
  ).toBeNull();
  expect(container.querySelector("script")).toBeNull();
  expect(
    inferenceAttempts.getByText(/<button>danger<\/button>/),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByText(/<script>alert\("xss"\)<\/script>/),
  ).toBeTruthy();
});
