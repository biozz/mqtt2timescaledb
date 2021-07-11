# mqtt2timescaledb

A simple script to convert messages received through mqtt to SQL inserts for TimescaleDB (PostgreSQL).

It is designed for a particular use case. I have a set of sensors around my home and elsewhere and they post data to MQTT broker. I want to collect the data, put it into time series database and have an overview in Grafana.

The measurements are in format `location/room/sensor/measurement`. The payload is a float. These are converted into insert statements for each event and inserted directly into PostgreSQL instance, which has TimescaleDB plugin enabled.

## Usage

I deploy this script using Ansible and run inside a docker container. The image is built first and run like this:

```yaml
community.docker.docker_container:
  name: mqtt2timescaledb
  image: mqtt2timescaledb:latest
  state: started
  restart_policy: "unless-stopped"
  command: 
    - "./mqtt2timescaledb -mq {{ broker_url }} -db {{ database_url }}"
  networks:
    - name: "{{ my_network }}"
```

Sometimes you want to debug messages. In this case add `-vvv` flag and you will see all messages in stdout.
