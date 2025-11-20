SHELL := /bin/bash

CERTS_DIR=certs
CA_KEY=$(CERTS_DIR)/ca.key
CA_CERT=$(CERTS_DIR)/ca.crt
SERVER_KEY=$(CERTS_DIR)/server.key
SERVER_CSR=$(CERTS_DIR)/server.csr
SERVER_CERT=$(CERTS_DIR)/server.crt

PROTO_DIR=api/proto
OUT_DIR=api/gen

PROTO_FILES=$(PROTO_DIR)/lpaas/v1alpha1/job.proto

.PHONY: proto
proto:
	protoc \
	  -I $(PROTO_DIR) \
	  --go_out=$(OUT_DIR) --go_opt=paths=source_relative \
	  --go-grpc_out=$(OUT_DIR) --go-grpc_opt=paths=source_relative \
	  $(PROTO_DIR)/lpaas/v1alpha1/job.proto


.PHONY: ca-cert server-certs client-certs clean-certs

ca-cert:
	mkdir -p $(CERTS_DIR)
	openssl ecparam -name prime256v1 -genkey -noout -out $(CA_KEY)
	openssl req -x509 -new -key $(CA_KEY) -out $(CA_CERT) -days 365 \
		-subj "/CN=lpaas Root CA"

server-certs: $(CA_KEY) $(CA_CERT)
	mkdir -p $(CERTS_DIR)
	openssl ecparam -name prime256v1 -genkey -noout -out $(SERVER_KEY)
	openssl req -new -key $(SERVER_KEY) -out $(SERVER_CSR) \
		-subj "/CN=lpaas-server"
	openssl x509 -req -in $(SERVER_CSR) -CA $(CA_CERT) -CAkey $(CA_KEY) \
		-CAcreateserial -out $(SERVER_CERT) -days 365 -sha256 \
		-extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

client-certs: $(CA_KEY) $(CA_CERT)
	@if [ -z "$(USER)" ]; then \
		echo "Please provide USER=<username> (e.g., make client-certs USER=rohit)"; \
		exit 1; \
	fi
	mkdir -p $(CERTS_DIR)
	openssl ecparam -name prime256v1 -genkey -noout -out $(CERTS_DIR)/client.key
	openssl req -new -key $(CERTS_DIR)/client.key -out $(CERTS_DIR)/client.csr \
		-subj "/CN=$(USER)"
	openssl x509 -req -in $(CERTS_DIR)/client.csr -CA $(CA_CERT) -CAkey $(CA_KEY) \
		-CAcreateserial -out $(CERTS_DIR)/client.crt -days 365 -sha256
	@ls -1 $(CERTS_DIR)/client.*

clean-certs:
	rm -rf $(CERTS_DIR)
