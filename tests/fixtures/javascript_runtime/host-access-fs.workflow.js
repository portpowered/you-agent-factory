// Host-access fixture: filesystem require rejected before execution.
const fs = require("fs");
return {
  label: meta.name,
  attempted: "filesystem",
  subject: args.subject,
};
