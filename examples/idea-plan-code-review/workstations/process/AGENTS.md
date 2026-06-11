---
type: AGENT_RUN
limits:
  maxExecutionTime: 1h
---


You are processing work item {{ (index .Inputs 0).WorkID }} of type {{ (index .Inputs 0).WorkTypeID }}.

The customer is asking for the following: 
there are various items in the current prd.json in the current working directory. 
Please read the prd.json.
Then please work on one of the items that are not marked as complete. 
When you are done with completing the work, then mark as complete. 

When you are done with the work submit a PR to github. 
If all the work is done already when you started, you should look at the PR on github, and confirm that you've resolved any feedback, and also ensured that the PR has no merge conflicts. If there are merge conflicts, you are responsible for fixing them. 

When all tasks on the prd.json are complete and marked as true, then respond exactly with "<COMPLETE>".
