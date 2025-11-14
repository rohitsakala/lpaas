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

2. Start the GRPC server using the following command 

```
go run ./server/server.go
```

### TODO/Future Work

- Update RFD after implementation review.
- Add logging to the codebase.
- Add race tests to run with `-race`.
