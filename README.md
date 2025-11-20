# Lpaas

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
