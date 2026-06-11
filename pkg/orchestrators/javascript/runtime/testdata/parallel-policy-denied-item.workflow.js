// Policy denial fixture: one parallel child violates model allowlist.
return (async function () {
  const results = await parallel([
    {
      prompt: "allowed child",
      label: "allowed-child",
      model: "gpt-allowed",
    },
    {
      prompt: "denied child",
      label: "denied-child",
      model: "gpt-denied",
    },
  ]);
  return { results: results };
})();
