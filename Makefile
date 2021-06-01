DB_HOST ?= 127.0.0.1
DB_URI := postgres://postgres@$(DB_HOST):5432/dl
export DB_URI

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
MIGRATE_DIR := ./migrations
SERVICE := dateilager.server

.PHONY: install test build server client-update client-get health
.PHONY: k8s-clear k8s-build k8s-deploy k8s-client-update k8s-client-get k8s-health
.PHONY: migrate migrate-create

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go install github.com/grpc-ecosystem/grpc-health-probe
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate

test: DB_URI = postgres://postgres@$(DB_HOST):5432/dl_tests
test: migrate
	cd test && go test

pkg/pb/%.pb.go: pkg/pb/%.proto
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative $^

pkg/pb/%_grpc.pb.go: pkg/pb/%.proto
	protoc --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative $^

bin/%: cmd/%/main.go $(PKG_GO_FILES)
	go build -o $@ $<

build: pkg/pb/fs.pb.go pkg/pb/fs_grpc.pb.go bin/server bin/client

server: export PORT=:5051
server:
	go run cmd/server/main.go

client-update:
	go run cmd/client/main.go -project 1 -server localhost:5051 update $(file)

client-get:
	go run cmd/client/main.go -project 1 -server localhost:5051 get

health: export SERVER=localhost:5051
health:
	grpc-health-probe -addr $(SERVER)
	grpc-health-probe -addr $(SERVER) -service $(SERVICE)

k8s-clear:
	kubectl delete --all service
	kubectl delete --all pod --grace-period 0 --force

k8s-build: build
	podman build -f Dockerfile -t "dateilager:server"
	podman save -o /tmp/dateilager_server.tar --format oci-archive "dateilager:server"
	sudo ctr -n k8s.io images import /tmp/dateilager_server.tar

k8s-deploy: k8s-build
	kubectl apply -f k8s/pod.yaml
	kubectl apply -f k8s/service.yaml

k8s: k8s-clear k8s-build k8s-deploy

k8s-client-get: SERVER=$(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):5051
k8s-client-get:
	go run cmd/client/main.go -project 1 -server $(SERVER) get

k8s-client-update: SERVER=$(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):5051
k8s-client-update:
	go run cmd/client/main.go -project 1 -server $(SERVER) update $(file)

k8s-health: SERVER=$(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):5051
k8s-health:
	grpc-health-probe -addr $(SERVER)
	grpc-health-probe -addr $(SERVER) -service $(SERVICE)

migrate:
	migrate -database $(DB_URI)?sslmode=disable -path $(MIGRATE_DIR) up

migrate-create:
	mkdir -p $(MIGRATE_DIR)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(name)
