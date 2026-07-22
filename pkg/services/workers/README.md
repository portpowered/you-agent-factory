# package structure of the workers. 

The workers package is a bit too deep. The intended system should be broken down a bit. 

pkg/services/workesr

runner (core logic)
services/
    agents <- this is responsible for agents relevant packages
        prompting
        worktree
        providers
            agy
                agypty
        cliprovider
    model_workers <- this is for model invocation
        invocation
    local_workers
    script_workers

    
    ## global system services
    workstations
    providers <- central registry for all the model provider types across agents, models, scripts etc
    mocks <- root mocks that wire into the process entry behavior and system loading
    observability
        recordsings
        diagnostics