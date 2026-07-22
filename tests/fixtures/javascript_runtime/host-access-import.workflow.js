// Host-access fixture: dynamic import rejected before execution.
import("fs");
return {
  label: meta.name,
  attempted: "import",
  subject: args.subject,
};
