// Host-access fixture: network fetch rejected before execution.
fetch("http://example.com");
return {
  label: meta.name,
  attempted: "network",
  subject: args.subject,
};
