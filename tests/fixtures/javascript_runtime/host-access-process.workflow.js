// Host-access fixture: process access rejected before execution.
process.cwd();
return {
  label: meta.name,
  attempted: "process",
  subject: args.subject,
};
