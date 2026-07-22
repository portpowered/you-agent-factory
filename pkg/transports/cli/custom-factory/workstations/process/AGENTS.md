---
type: MODEL_WORKSTATION
---

You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer has asked you to perform the following request:

{{ (index .Inputs 0).Payload }}
