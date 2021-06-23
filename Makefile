PROJECT := dateilager

DB_HOST ?= 127.0.0.1
DB_URI := postgres://postgres@$(DB_HOST):5432/dl

GRPC_PORT ?= 5051
GRPC_SERVER ?= localhost:$(GRPC_PORT)

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
INTERNAL_GO_FILES := $(shell find internal/ -type f -name '*.go')

MIGRATE_DIR := ./migrations
SERVICE := $(PROJECT).server

.PHONY: install migrate migrate-create build test
.PHONY: reset-db setup-local server client-update client-get client-rebuild client-pack health
.PHONY: k8s-clear k8s-build k8s-deploy k8s-client-update k8s-client-get k8s-client-rebuild k8s-client-pack k8s-health

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

internal/pb/%.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative $^

internal/pb/%_grpc.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative $^

bin/%: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES)
	go build -o $@ $<

build: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go bin/server bin/client

test: export DB_URI = postgres://postgres@$(DB_HOST):5432/dl_tests
test: migrate
	cd test && go test

reset-db: migrate
	psql $(DB_URI) -c "truncate dl.objects; truncate dl.contents; truncate dl.projects; insert into dl.projects (id, latest_version) values (1, 0);"

setup-local: reset-db
	scripts/simple_input.sh

server:
	go run cmd/server/main.go -dburi $(DB_URI) -port $(GRPC_PORT)

client-update:
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/initial.txt -directory input/v1
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/diff_v1_v2.txt -directory input/v2
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/diff_v2_v3.txt -directory input/v3

client-get:
ifndef version
	go run cmd/client/main.go get -project 1 -server $(GRPC_SERVER) -prefix "$(prefix)"
else
	go run cmd/client/main.go get -project 1 -server $(GRPC_SERVER) -to $(version) -prefix "$(prefix)"
endif

client-rebuild:
ifndef version
	go run cmd/client/main.go rebuild -project 1 -server $(GRPC_SERVER) -prefix "$(prefix)" -output $(output)
else
	go run cmd/client/main.go rebuild -project 1 -server $(GRPC_SERVER) -to $(version) -prefix "$(prefix)" -output $(output)
endif

client-pack:
	go run cmd/client/main.go pack -project 1 -server $(GRPC_SERVER) -path $(path)

health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)

k8s-clear:
	kubectl -n $(PROJECT) delete --all service
	kubectl -n $(PROJECT) delete --all pod --grace-period 0 --force
	kubectl -n $(PROJECT) delete --all configmap
	kubectl delete ns $(PROJECT)

k8s-build: build
	podman build -f Dockerfile -t "$(PROJECT):server"
	podman save -o /tmp/$(PROJECT)_server.tar --format oci-archive "$(PROJECT):server"
	sudo ctr -n k8s.io images import /tmp/$(PROJECT)_server.tar

k8s-deploy: k8s-build
	kubectl create -f k8s/namespace.yaml
	kubectl -n $(PROJECT) create configmap server-config --from-env-file=k8s/server.properties
	kubectl -n $(PROJECT) apply -f k8s/pod.yaml
	kubectl -n $(PROJECT) apply -f k8s/service.yaml

k8s: k8s-clear k8s-build k8s-deploy

k8s-client-update: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-update: client-update

k8s-client-get: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-get: client-get

k8s-client-rebuild: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-rebuild: client-rebuild

k8s-client-pack: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-pack: client-rebuild

k8s-health: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)
