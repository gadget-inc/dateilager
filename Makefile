DB_HOST ?= 127.0.0.1
DB_URI := postgres://postgres@$(DB_HOST):5432/dl

GRPC_PORT ?= 5051
GRPC_SERVER := localhost:$(GRPC_PORT)

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
MIGRATE_DIR := ./migrations
SERVICE := dateilager.server

.PHONY: install migrate migrate-create build test
.PHONY: reset-db setup-local server client-update client-get health
.PHONY: k8s-clear k8s-build k8s-deploy k8s-client-update k8s-client-get k8s-health

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go install github.com/grpc-ecosystem/grpc-health-probe
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate

migrate:
	migrate -database $(DB_URI)?sslmode=disable -path $(MIGRATE_DIR) up

migrate-create:
	mkdir -p $(MIGRATE_DIR)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(name)

internal/pb/%.pb.go: pkg/pb/%.proto
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative $^

internal/pb/%_grpc.pb.go: pkg/pb/%.proto
	protoc --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative $^

bin/%: cmd/%/main.go $(PKG_GO_FILES)
	go build -o $@ $<

build: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go bin/server bin/client

test: export DB_URI = postgres://postgres@$(DB_HOST):5432/dl_tests
test: migrate
	cd test && go test

reset-db: migrate
	psql $(DB_URI) -c "truncate dl.objects; truncate dl.contents; update dl.projects set latest_version = 0 where id = 1;"

setup-local: reset-db
	scripts/simple_input.sh

server:
	go run cmd/server/main.go -dburi $(DB_URI) -port $(GRPC_PORT)

client-update:
	go run cmd/client/main.go -project 1 -server $(GRPC_SERVER) update input/initial.txt input/v1
	go run cmd/client/main.go -project 1 -server $(GRPC_SERVER) update input/diff_v1_v2.txt input/v2
	go run cmd/client/main.go -project 1 -server $(GRPC_SERVER) update input/diff_v2_v3.txt input/v3

client-get:
	go run cmd/client/main.go -project 1 -server $(GRPC_SERVER) get $(prefix)

health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)

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

k8s-client-get: SERVER = $(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-get:
	go run cmd/client/main.go -project 1 -server $(SERVER) get

k8s-client-update: SERVER = $(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-update:
	go run cmd/client/main.go -project 1 -server $(SERVER) update $(file)

k8s-health: SERVER = $(shell kubectl get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-health:
	grpc-health-probe -addr $(SERVER)
	grpc-health-probe -addr $(SERVER) -service $(SERVICE)
