// Simple final-only workflow fixture that completes through workflow.final.
workflow.final({
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
  mechanism: "workflow.final",
});
