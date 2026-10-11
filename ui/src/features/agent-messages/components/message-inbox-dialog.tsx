import { useState } from "react";
import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "../../../components/ui/dialog";
import type { NavigateMessageWorker } from "../hooks/use-message-thread";
import { getAgentMessageMessages } from "../messages/agent-messages";
import { MessageInbox } from "./message-inbox";

export function MessageInboxDialog({
  locale,
  onNavigate,
}: {
  locale?: string;
  onNavigate: NavigateMessageWorker;
}) {
  const [open, setOpen] = useState(false);
  const copy = getAgentMessageMessages(locale);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <DashboardActionButton type="button">
          {copy.inbox}
        </DashboardActionButton>
      </DialogTrigger>
      <DialogContent
        closeLabel={copy.close}
        className="max-h-dvh overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>{copy.inbox}</DialogTitle>
          <DialogDescription>{copy.inboxDescription}</DialogDescription>
        </DialogHeader>
        {open ? (
          <MessageInbox
            locale={locale}
            onNavigate={(identity) => {
              setOpen(false);
              onNavigate(identity);
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
