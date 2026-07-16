Complete the requested work carefully.

Request:
{{ (index .Inputs 0).Payload }}

{{ if gt (index .Inputs 0).History.AttemptNumber 1 -}}
Previous rejected work:
{{ (index .Inputs 0).PreviousOutput }}

Reviewer feedback:
{{ (index .Inputs 0).RejectionFeedback }}

{{ end -}}
Return the completed work as your final response. Do not claim it is reviewed.
