# Graylog GELF Module for Logspout
This module allows [Logspout](https://github.com/gliderlabs/logspout) to send Docker logs in the GELF format to Graylog via UDP.
Also can parse default [Monolog](https://github.com/Seldaek/monolog) message format. Not formatted message fully ps
assed to GELF short_message

## Build & Run

See [official Logspout custom build documentation](https://github.com/gliderlabs/logspout/tree/master/custom)

## A note about GELF parameters
The following docker container attributes are mapped to the corresponding GELF extra attributes.

```
{
        "_container_id":   <container-id>,
        "_container_name": <container-name>,
        "_image_id":       <container-image-sha>,
        "_image_name":     <container-image-name>,
        "_command":        <container-cmd>,
        "_created":        <container-created-date>,
        "_swarm_node":     <host-if-running-on-swarm>
}
```

You can also add extra custom fields by adding labels to the containers.

for example 
a container with label ```gelf_service=servicename``` will have the extra field service

### Message format
```
[timestamp] facility.level: short context extra  
```
where:

**_timestamp_**: ISO 8601 with microseconds or no. 

_examples:_
```
2020-04-23T06:56:51.092957-03:00
2020-04-23T06:56:51+04:00
``` 

**_facility_**: symbols set from `a-zA-Z_-`. Pass to GELF facility as is

**_level_**: log level `(DEBUG|INFO|NOTICE|WARNING|ERROR|CRITICAL|ALERT|EMERGENCY)`. Converts to GELF level

**_short_**: log message. Pass to GELF short_message as is

**_context_**, **_extra_**: single level JSON-formatted fields. First level converts to GELF additional. 

For example:
```
[2020-04-30T12:03:13+0000] php-fpm.INFO: GET /url/ 200 {"CPU":24.89,"request_time_us":1607322,"memory_b":27721728} []
```
will converts to GELF
```json
{
  "version":"1.1",
  "host":"local",                 
  "short_message":"GET /url/ 200",
  "timestamp":1588248193590,
  "level":6,
  "facility":"php-fpm",
  "_CPU":24.89,
  "_memory_b":27721728,
  "_request_time_us":1607322,
  "_command":"php-fpm",
  "_container_name":"webapp",
  "_created":"2019-06-28T14:31:04.2618359Z",
  "_container_id":"12daf3d74812db16c63886b1c7e4c1f5251eabe48c846a72434d96a6ea3a4e38",
  "_image_id":"sha256:d5b5d4bb20a7cfbaedea01e84eec24d190ef06db53915e4022be08539577c3dc",
  "_image_name":"webapp_image"
}
```

### Environment variables

COMPRESS_TYPE: none|gzip|zlib

COMPRESS_LEVEL: -1..9; -1 - default compress level

EXTRA_JSON: json formatted extra additional GELF params; First underscore will be added by module


## License
MIT. See [License](LICENSE)
