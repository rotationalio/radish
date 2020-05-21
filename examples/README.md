# Getting Started

The `examples` folder contains `turnip`, an example of a service that implements the radish API to illustrate how to use radish for concurrent task monitoring inside the context of a real application. Using `turnip` as a demo, you can experiment with adding different types of tasks, scaling up and down the number of concurrent workers, and visualizing the resulting prometheus metrics.

## Running the container

After installing [Docker Desktop for Mac](https://www.docker.com/products/docker-desktop), run docker from inside the `examples` directory:

    ```bash
    $ docker-compose up
    ```

You'll now be running three services:
- Turnip: a demo of using the radish API implemented in the `turnip` directory, which is serving metrics on port 9090.
- Prometheus: configured by the `prometheus.yml` file, which will scrape for prometheus metrics every second.
- Grafana: a time series visualization and monitoring UI that will scrape Prometheus every second and allow us to visualize our metrics.


## Interacting with the Radish API
With the Turnip server running, you'll now be able to interact with the radish API,

e.g. to see what kind of task types are available:

```bash
$ radish status
```

Or to queue up short tasks:
```bash
$ radish -U queue -t short
```

Long tasks:
```bash
$ radish -U queue -t long
```

Or tasks that have a high probability of failure:
```bash
$ radish -U queue -t chance
```

Or to scale up or scale down the number of concurrent workers to handle those tasks:
```bash
$ radish -U scale -w 5
```

Running commands such as those above will generate updates to the prometheus metrics, which will be visible in the Grafana dashboard, once you've configured it...

## Configuring the dashboard

In the browser navigate to Grafana, which is at [`http://localhost:3000`](http://localhost:3000) and sign in with the default login and password, both of which are "admin". You'll be prompted to update your password.

Next create a data source; select the Prometheus option; under the `HTTP` header, change the URL to `http://prometheus:9090` and set the HTTP method to `GET`; save and test.

Next navigate back to the main Grafana page to build a dashboard. Choose `Add Query`, under `Metrics`, try adding a sample metric, such as a sum of the workers in the queue, `scalar(radish_queue_workers)`. Metrics will be named `radish_queue_metric_name` i.e. the namespace_subsystem_metric and then labels can be accessed with a `.` property.

## Create a simulation

There won't be too much going on in the dashboard unless we are running commands that actively change the numbers of tasks and workers. Here's a simple simulation you can run in order to generate some activity:

```bash
#!/bin/bash
while true; do
    radish -U queue -t chance || break
    sleep .1
    radish -U queue -t long || break
    sleep .2
done
```