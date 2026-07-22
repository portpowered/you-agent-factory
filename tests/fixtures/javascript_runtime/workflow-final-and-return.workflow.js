// Fixture that both returns and calls workflow.final to prove deterministic selection.
workflow.final({
  label: meta.name,
  mechanism: "workflow.final",
  subject: args.subject,
});
return {
  label: meta.name,
  mechanism: "return",
  subject: args.subject,
};
