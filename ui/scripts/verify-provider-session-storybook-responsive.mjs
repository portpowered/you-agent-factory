export async function verifyProviderSessionDetailSuccess({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const providerSessionDetails = page.getByRole("region", {
    name: "Selected session details",
  });
  await expectVisible(providerSessionDetails, "Provider-session details region");
  await expectVisible(
    providerSessionDetails.getByRole("heading", {
      name: "Selected session details",
    }),
    "Selected session details heading",
  );
  await expectVisible(
    providerSessionDetails.getByRole("heading", { name: "Source file" }),
    "Provider-session source file heading",
  );
  await expectVisible(
    providerSessionDetails.getByText(
      "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
    ),
    "Timestamp-prefixed provider-session source path",
  );
  await expectVisible(
    providerSessionDetails.getByRole("heading", { name: "Token usage" }),
    "Provider-session token usage heading",
  );
  await expectNoHorizontalOverflow(
    page,
    `Provider-session detail success at ${viewport.label}`,
  );
}
