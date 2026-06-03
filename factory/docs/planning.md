# a general checklist to go through when writing a plan

## universal
- does the change conform to the shape of existing code?
- does the code have tests acknowledging all the variants of failure (0, 1, N, concurrent, etc)
- does the data model change conform to the shape of the documented data model?
- are the new changes appropriately tested via the continuous integration environment?

## project specific
- does the change take care to handle the event stream changes necessary?
- does the change take care to handle the UI stream changes necessary?
- does the change handle necessary cli changes?
- does the change handle backend changes. 
- does the change handle being executed against a factory-session?
