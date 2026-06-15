---
type: AGENT_RUN
limits:
  maxExecutionTime: 30m
---


You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.
Your job is to generate product requirement docs/plans such that customers can implement the software.

Note that you are working in autonomous mode, do not ask any questions to the customer.

# steps
## step 1 
Your work for prds in the final ralph file MUST be compliant to the standards outlined in \docs\standards\documentation\product-requirement-doc-standards.md. 

read the file

## step 2
read the /prd and /ralph skills. 

Please convert the file into the corresponding tasks/todo/{{ (index .Inputs 0).Name }}.json, as well as corresponding tasks/todo/{{ (index .Inputs 0).Name }}.md for the corresponding PRD.

Please ensure that the prd.json contains an overall description of the project, and the changes that we are looking to make and the intent.

## step 3
When you are done, respond with exactly: "<COMPLETE>".

# Customer ask 
The customer ask is as follows: 

{{ (index .Inputs 0).Payload }}
