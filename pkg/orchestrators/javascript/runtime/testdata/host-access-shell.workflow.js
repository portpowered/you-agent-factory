// Host-access fixture: shell/process spawn rejected before execution.
child_process.exec("echo denied");
return {
  label: meta.name,
  attempted: "shell",
  subject: args.subject,
};
