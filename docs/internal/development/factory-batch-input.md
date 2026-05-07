Customers are able to submit unary requests to the agenet factory today, but sometimes neeed to submit a request group in batch wherein the new request items take a dependency on each other, or can be computed in parallel. 

To enable this we look to enable customers to modify the factory APIs/internal runtime to take in a new object we call a WorkRequest, as defined in the current openapi.yaml for the agent-factory. 

## customer experience

with the work request, customers take an input of a workrequest, and inside of that workrequest, the customer declares for their input a set of work, and the relations of the inputs of the work. 

For example: 

bob wants to start a big project, that has a bunch of separate items that need to be started up at the same time, with some in parallel, but some having dependency on yet other work. 
Bob enables the work to start. 
Bob writes the file that declares the workrequest, and puts it into the factory inputs folder for say the "tasks" folder.
The factory is watching files in the folder, and then consumes the request. 
The factory goes along running the work request as a whole, while keeping in mind the relationships of the work inputs. 

i.e. 
```json
{
    "type": "FACTORY_REQUEST_BATCH",
    "works": [{
        "name": "first",
        "payload": "something-somthing"
    }, {
        "name": "second",
        "payload": "something-else"
    }],
    "relations": [
        {
            "type": "DEPENDS_ON",
            "source_work_name": "second",
            "target_work_name": "first"
        }
    ]

}
```

Then bob just waits until it completes. 

## gotchas
- note that the work submitted is presumed to be of the same type for now, we dont' have any particular constraint on this on the API side, which rejects if the API does not have declare worked types for each input, but for the filewatcher, the work type is presumed from the workchannel, and there's no default general input channel from that filewatcher so until we come up with one, just presume that doesn't exist. 


## Work items
- the factory engine should be extended with a new API called batchSubmitRequest, which we will make all the logic currently route into. i.e. the unary submitrequest should be pointed into the batchSubmit
- the filewatcher shoud watch for files that match that factory_request_batch structure on input, and if it does match, submit that request to the batch
- the current event stream that watches for events should be updated to have the events in history/state to match teh batch request structure, so as to model a request batch input, similarly for the history
- the website should be updated to handle this new change to the event stream
- replace legacy structured input handling with canonical factory request batch handling.
- use the generated code interfaces from the openapi for the parsing of request inputs from the files as well as from the API, that way we can have a unary way to deserialize and normalize request payloaads. 
- confirm the functional tests have been rewritten to handle the request batch, confirming the relational constraints of the batch are maintained, such that element A, is completed before element B based on the relations graph.
- replace legacy decomposing of batch requests into unary requests with the aggregate structure input.
