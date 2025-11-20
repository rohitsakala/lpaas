# Lpaas gravitational teleport challenge Level 5

## Level 5

### Library

* Worker library with methods to start/stop/query status of a job.
  * When stopping a job, care should be taken to ensure that the job's child processes
    (if any) are also terminated.
* Library should be able to stream the output of a running job.
  * Discovering new output should be efficient, avoid busy-waiting or polling.
  * Output should be from start of process execution.
  * Multiple concurrent clients should be supported.
  * Do not make any assumptions about the process's output - it may be text or raw binary data.
* Add resource control for CPU, Memory and Disk IO per job using cgroups.

### API

* [GRPC](https://grpc.io) API to start/stop/get status/stream output of a running process.
* Use mTLS authentication and verify client certificate. Set up strong set of
  cipher suites for TLS and good crypto setup for certificates. Do not use any
  other authentication protocols on top of mTLS.
* Use a simple authorization scheme.

### Building Proto

1. Using the following versions on a machine

```
protoc-gen-go v1.36.10
protoc        v3.21.12
```

2. Run the following make target

```
make proto
```

### Running the project 

1. First step is to generate the certs 

```
make ca-cert
make server-certs
make client-certs USER=rohit
```

2. Start the GRPC server using the following command using root

```
go run ./server/server.go
```

3. Build the client by the make target using root

```
make build-client
```

4. Later, you can use the `./bin/lpass-client` as client.

```
./bin/lpass-client --help
```

Note: This project can only be run on linux machines with cgroup (cpu io mem enabled) version 2.

### TODO/Future Work

- Update RFD after implementation and implementation code review.
- Add logging to the codebase.
- Add race tests to run with `-race`.
- Add `UnHappy` integration tests.
- Currently this project only runs on Linux with cgroupv2 and `findmnt` command.
