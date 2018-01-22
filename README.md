# bitflow-process-agent

This agent executes and manages subprocesses through a REST API. More specifically, it manages instances of [bitflow-pipeline](https://gitlab.tubit.tu-berlin.de/anton.gulenko/go-bitflow-pipeline/tree/master/bitflow-pipeline) (also forked [here](https://gitlab.tubit.tu-berlin.de/cit-master-project/go-bitflow-pipeline)) that execute stream processing pipelines defined by [Bitflow Script](https://gitlab.tubit.tu-berlin.de/anton.gulenko/go-bitflow-pipeline/tree/master/query#bitflow-script).

When starting, the agent performs the following steps:
1. Query and store the capabilities of the used `bitflow-pipeline` executable. Invoke the executable with the `-capabilities` flag and parse the JSON output. Exit if the command fails or the output cannot be properly parsed. See the REST API section below for an example of the expected JSON format (the same format is served through the REST API).
2. Initialize the HTTP server for the REST API. Exit if creating the listening socket fails.
3. Optionally notify a manager instance by issuing one HTTP GET request to a specified URL (see command line flags below). Exit if the GET request fails, times out, or does not return a 200 status code. The purpose of this step is to enable automatic, scalable discovery of agents by a centralized manager instance that will use the provided REST API.

After this startup sequence, the agent waits for incoming HTTP requests and manages the life cycle of the created subprocesses. Check the Current Limitations section at the end of this README.

## Command line flags
All flags are optional (see the default values).

**`-e file`** Name of the pipeline executable. By default, search `$PATH` for `bitflow-pipeline`.

**`-h tcp-endpoint`** HTTP endpoint for serving REST API (default `:8080`).

**`-m url`** After initializing the REST API, send a GET request with an empty body and no further headers to the given URL. Any parameters or options needed by the manager must be encoded in query parameters (e.g. `-m 'http://manager.com/registeragent?ip=10.0.0.1&port=5555'`).

**`-tag key=value`** Additional key=value pairs that will be served through GET /info.

## REST API

In general, the status code `404` is returned when an unexpected path or request verb is used. Whenever the status code is not success (`200` or `201`), the response will be an unformatted string explaining the error. The response format of the success status codes depends on the API function, but is usually unindented JSON.

##### `GET /ping`
Returns the string `pong`. Can be used as a low-overhead alive-test.

Status code: `200`.

##### `GET /info`
Returns information about the agent and the host. Both static and dynamically changing information is served.
The returned `Tags` property is filled from the `-tag` command line flags defined when starting the agent.
The tags can be used to classify or tag the host type, used by the end-user or for scheduling decisions.

Status code: `200`.

Example response body:
```
{
  "Hostname": "worker12",
  "Tags": {
    "resources": "medium",
    "slots": "6"
  },
  "NumCores": 4,
  "TotalMem": 8254799872,
  "UsedCpuCores": [
    3.960396039350753,
    2.970297029767164,
    2.0000000000436557,
    2.0202020202428503
  ],
  "UsedCpu": 2.7377237723511056,
  "UsedMem": 6126739456,
  "NumProcs": 247,
  "Goroutines": 6
}
```

##### `GET /capabilities`
Return the capabilities of the managed `bitflow-pipeline` executable. The returned value is the same that is printed when executing `bitflow-pipeline -capabilities`. The JSON structure contains all pipeline processing steps that can be used in the [Bitflow Script](https://gitlab.tubit.tu-berlin.de/anton.gulenko/go-bitflow-pipeline/tree/master/query#bitflow-script) when starting a new pipeline instance.

Note on the `OptionalParams` and `RequiredParams` properties: If `OptionalParams` is `null`, it is the same as an empty list. However, if both properties are `null`, it means that the processing step accepts arbitrary parameters.

Status code: `200`.

Example response body:
```
[
  {
    "Name": "avg",
    "IsFork": false,
    "Description": "Add an average metric for every incoming metric. Optional parameter: duration or number of samples. Optional parameters: [window]",
    "RequiredParams": [],
    "OptionalParams": [
      "window"
    ]
  },
  {
    "Name": "pick",
    "IsFork": false,
    "Description": "Forward only a percentage of samples, parameter is in the range 0..1. Required parameters: [percent]",
    "RequiredParams": [
      "percent"
    ],
    "OptionalParams": null
  },
  {
    "Name": "tags",
    "IsFork": false,
    "Description": "Set the given tags on every sample. Variable parameters",
    "RequiredParams": null,
    "OptionalParams": null
  }
]
```

##### `GET /pipelines`
Return a list of IDs of all pipelines in all states, including failed, finished and killed pipelines (see `GET /pipeline/:id` for possible pipeline states).

Status code: `200`.

Example response body:
```
[0,1,2,3,4,5]
```

##### `GET /running`
Return a list of IDs of all currently running pipelines.

Status code: `200`.

Example response body:
```
[3,5]
```

##### `POST /pipeline[?delay=200ms&params=xxx]`
Create a new pipeline subprocess. The id of the pipeline will be assigned automatically. The body of the POST request is entirely used as the [Bitflow Script](https://gitlab.tubit.tu-berlin.de/anton.gulenko/go-bitflow-pipeline/tree/master/query#bitflow-script). The `delay` query parameter is optional, the default value is `200ms`. It is parsed by [time.ParseDuration](https://golang.org/pkg/time/#ParseDuration). If the `delay` value is greater than zero, the server waits for the given interval after spawning the subprocess before sending the HTTP response. If the subprocess exits abnormally before the given interval, the response will contain the combined standard output and standard error of the subprocess. The `params` query parameter is also optional. It can be provided to pass parameters to the resulting pipeline process. The possible parameters depend on the actual pipeline executable, wrong parameters will likely prevent the pipeline from starting. The value of the `params` query parameter is parsed by [shellquote.Split](https://godoc.org/github.com/kballard/go-shellquote#Split), which splits the string into individual parameters following the rules of `/bin/sh`, including single quotes, double quotes and backslash escapes. This way multiple parameters can be passed through a single query parameter value.

Status code: `201`
* If the subprocess is spawned successfully and does not fail early.

Example response body:
```
{"Id":10,"Script":"localhost:4444 -> output.csv","Status":"running","Errors":""}
```
Note: the format of the response body is the same as in the `GET /pipeline/:id` API call.

Status code: `400`
* If the request body is empty.
* If the `delay` parameter cannot be parsed

Status code: `412`
* If the subprocess cannot be spawned or exits within the defined `delay`. In the latter case, the response body will also contain the combined standard output and standard error of the process.

Status code: `500`
* If the server fails to read the request body

##### `GET /pipeline/:id`
Return a JSON formatted view of the given pipeline. The `Errors` property can contain hints about how the current `Status` of the pipeline was reached, but usually the `GET /pipeline/:id/out` function provides more useful insights.

The `Status` property describes the state of the pipeline, it can take the following values:
* `"created"`: The pipeline has been created and not yet started. Will usually not be observed, as every pipeline is immediately started after creation.
* `"running"`: The pipeline has been successfully started and is currently executing.
* `"finished"`: The pipeline subprocess finished with a zero exit code.
* `"failed"`: The pipeline subprocess could not be successfully created, or failed with a non-zero exit code.
* `"killing"`: The `DELETE /pipeline/:id` function was used to kill the pipeline. The subprocess has not yet exited. If the pipeline remains in this state, a manual cleanup (e.g. `kill -9`) could be necessary.
* `"killed"`: The `DELETE /pipeline/:id` function was used to kill the pipeline, and the subprocess exited.

Status code: `200`.

Example response body:
```
{"Id":0,"Script":"10.0.0.1:5000 -> avg() -> :5001","Status":"running","Errors":""}
```

Example response body:
```
{"Id":2,"Script":":1","ExtraParams":[],"Status":"failed","Errors":"exit status 1"}
```

Status code: `400`
* If the `:id` part of the URL cannot be parsed to an integer.

Status code: `404`
* If the given pipeline id does not exist.

##### `GET /pipeline/:id/out`
Return the combined standard output and standard error of the given pipeline.

Status code: `200`.

Example response body:
```
INFO[Nov  8 15:14:44.025] Pipeline                                     
INFO[Nov  8 15:14:44.025] ├─TCP source on :5000                        
INFO[Nov  8 15:14:44.025] ├─Feature Aggregator [_avg]                  
INFO[Nov  8 15:14:44.025] └─TCP sink on :5001                          
INFO[Nov  8 15:14:44.025] Listening for output connections on [::]:5001  format=binary
INFO[Nov  8 15:14:44.025] Listening for incoming data on [::]:5000      format=auto-detected
```

Status code: `400`
* If the `:id` part of the URL cannot be parsed to an integer.

Status code: `404`
* If the given pipeline id does not exist.

Status code: `500`
* If the output of the pipeline could not be obtained.

##### `DELETE /pipeline/:id`
Try to kill the given pipeline.

Status code `200`:
* If the pipeline was successfully killed. The response body will be the state of the pipeline, as returned by `GET /pipeline/:id`.

Status code: `400`
* If the `:id` part of the URL cannot be parsed to an integer.

Status code: `404`
* If the given pipeline id does not exist.

Status code: `500`.
* If the subprocess could not be killed. In this case the subprocess might still be running and might require manual cleanup (see Current limitations).

## Current limitations
* No persistency. When the agent is restarted, it forgets about previously running subprocesses. If they are still running, they must be killed externally.
* Limited process management. If a subprocess does not terminate normally (after a SIGKILL), cleaning up the subprocess is not further enforced.
* Leaking memory. The output and metadata of every started subprocess is stored in memory indefinitely.
