import { findSubmitWorkCard } from "./submit-work-card-queries";

describe("findSubmitWorkCard", () => {
  it("queries the submit work card article by role", async () => {
    const article = document.createElement("article");
    const scope = {
      getByRole: vi.fn(),
      findByRole: vi.fn().mockResolvedValue(article),
    };

    await expect(findSubmitWorkCard(scope)).resolves.toBe(article);
    expect(scope.findByRole).toHaveBeenCalledWith("article", {
      name: "Submit work",
    });
  });
});
// Component lane: requires DOM APIs.
