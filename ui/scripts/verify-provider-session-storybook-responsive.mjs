export async function verifyProviderSessionDetailSuccess({
  expectNoHorizontalOverflow,
  expectVisible,
  page,
  viewport,
}) {
  const providerSessionHeading = page.getByRole("heading", {
    name: "Selected session details",
  });
  await expectVisible(
    providerSessionHeading,
    "Selected session details heading",
  );
  await expectVisible(
    page.getByRole("heading", { name: "Source file" }),
    "Provider-session source file heading",
  );
  await expectVisible(
    page.getByText(
      "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
    ),
    "Timestamp-prefixed provider-session source path",
  );
  await expectVisible(
    page.getByRole("heading", { name: "Token usage" }),
    "Provider-session token usage heading",
  );
  await expectNoHorizontalOverflow(
    page,
    `Provider-session detail success at ${viewport.label}`,
  );
}
