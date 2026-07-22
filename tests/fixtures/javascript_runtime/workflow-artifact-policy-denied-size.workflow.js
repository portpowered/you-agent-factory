// Policy denial fixture: artifact content exceeds maxArtifactBytes.
workflow.artifact({
  kind: "report",
  label: "oversized",
  content: { body: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" },
});
return { ok: false };
