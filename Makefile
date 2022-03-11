PROJECT := dateilager

DB_HOST ?= 127.0.0.1
DB_USER ?= postgres
DB_URI := postgres://$(DB_USER)@$(DB_HOST):5432/dl

GRPC_PORT ?= 5051
GRPC_SERVER ?= localhost:$(GRPC_PORT)

DEV_TOKEN_ADMIN ?= v2.public.eyJzdWIiOiJhZG1pbiIsImlhdCI6IjIwMjEtMTAtMTVUMTE6MjA6MDAuMDM0WiJ9WtEey8KfQQRy21xoHq1C5KQatEevk8RxS47k4bRfMwVCPHumZmVuk6ADcfDHTmSnMtEGfFXdxnYOhRP6Clb_Dw
DEV_TOKEN_PROJECT_1 ?= v2.public.eyJzdWIiOiIxIiwiaWF0IjoiMjAyMS0xMC0xNVQxMToyMDowMC4wMzVaIn2MQ14RfIGpoEycCuvRu9J3CZp6PppUXf5l5w8uKKydN3C31z6f6GgOEPNcnwODqBnX7Pjarpz4i2uzWEqLgQYD

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
INTERNAL_GO_FILES := $(shell find internal/ -type f -name '*.go')

MIGRATE_DIR := ./migrations
SERVICE := $(PROJECT).server

.PHONY: install migrate migrate-create build release test
.PHONY: reset-db setup-local server server-profile client-update client-get client-rebuild client-pack health
.PHONY: k8s-clear k8s-build k8s-deploy k8s-client-update k8s-client-get k8s-client-rebuild k8s-client-pack k8s-health
.PHONY: upload-container-image

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1
	go install github.com/grpc-ecosystem/grpc-health-probe@v0.4
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.14
	go install github.com/bojand/ghz/cmd/ghz@v0.105.0
	go install github.com/gadget-inc/fsdiff/cmd/fsdiff@v0.1
	cd js && npm install

migrate:
	migrate -database $(DB_URI)?sslmode=disable -path $(MIGRATE_DIR) up

migrate-create:
ifndef name
	$(error name variable must be set)
else
	mkdir -p $(MIGRATE_DIR)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(name)
endif

internal/pb/%.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative $^

internal/pb/%_grpc.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative $^

bin/%: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 go build -o $@ $<

js/src/%.client.ts: internal/pb/%.proto
	cd js && npx protoc --experimental_allow_proto3_optional --ts_out ./src --ts_opt long_type_bigint --proto_path ../internal/pb/ ../$^

dev/server.key:
	mkcert -cert-file dev/server.crt -key-file dev/server.key localhost

dev/server.cert: dev/server.key

build: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go bin/server bin/client bin/webui js/src/fs.client.ts dev/server.cert

release/%_linux_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ $<

release/%_macos_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $@ $<

release/%_macos_arm64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $@ $<

release/migrations.tar.gz: migrations/*
	tar -zcf $@ migrations

release/assets.tar.gz: assets/*
	tar -zcf $@ assets

release: build
release: release/server_linux_amd64 release/server_macos_amd64 release/server_macos_arm64
release: release/client_linux_amd64 release/client_macos_amd64 release/client_macos_arm64
release: release/webui_linux_amd64 release/webui_macos_amd64 release/webui_macos_arm64
release: release/assets.tar.gz release/migrations.tar.gz

test: export DB_URI = postgres://$(DB_USER)@$(DB_HOST):5432/dl_tests
test: migrate
	cd test && go test

reset-db: migrate
	psql $(DB_URI) -c "truncate dl.objects; truncate dl.contents; truncate dl.projects;"

setup-local: reset-db
	psql $(DB_URI) -c "insert into dl.projects (id, latest_version, pack_patterns) values (1, 0, '{\"node_modules/.*/\"}');"
	scripts/simple_input.sh

server: export DL_ENV=dev
server:
	go run cmd/server/main.go -dburi $(DB_URI) -port $(GRPC_PORT)

server-profile: export DL_ENV=dev
server-profile:
	go run cmd/server/main.go -dburi $(DB_URI) -port $(GRPC_PORT) -prof cpu.prof -log info

client-update: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-update:
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/v1_state/diff.s2 -directory input/v1
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/v2_state/diff.s2 -directory input/v2
	go run cmd/client/main.go update -project 1 -server $(GRPC_SERVER) -diff input/v3_state/diff.s2 -directory input/v3

client-get: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-get:
ifndef version
	go run cmd/client/main.go get -project 1 -server $(GRPC_SERVER) -prefix "$(prefix)"
else
	go run cmd/client/main.go get -project 1 -server $(GRPC_SERVER) -to $(version) -prefix "$(prefix)"
endif

client-rebuild: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-rebuild:
ifndef version
	go run cmd/client/main.go rebuild -project 1 -server $(GRPC_SERVER) -prefix "$(prefix)" -output $(output)
else
	go run cmd/client/main.go rebuild -project 1 -server $(GRPC_SERVER) -to $(version) -prefix "$(prefix)" -output $(output)
endif

webui: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
webui:
	go run cmd/webui/main.go -server $(GRPC_SERVER)

health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)

k8s-clear:
	kubectl -n $(PROJECT) delete --all service
	kubectl -n $(PROJECT) delete --all pod --grace-period 0 --force
	kubectl -n $(PROJECT) delete --all configmap
	kubectl delete ns $(PROJECT) || true

k8s-build: build
	docker build -f Dockerfile -t "localhost/$(PROJECT):server" .
	docker save -o /tmp/$(PROJECT)_server.tar "localhost/$(PROJECT):server"
	sudo ctr -n k8s.io images import /tmp/$(PROJECT)_server.tar

k8s/server.properties:
	scripts/generate_k8s_config.sh $(DB_URI)

k8s-deploy: k8s/server.properties k8s-build
	kubectl create -f k8s/namespace.yaml
	kubectl -n $(PROJECT) create secret tls server-tls --cert=dev/server.crt --key=dev/server.key
	kubectl -n $(PROJECT) create secret generic server-paseto --from-file=dev/paseto.pub
	kubectl -n $(PROJECT) create configmap server-config --from-env-file=k8s/server.properties
	kubectl -n $(PROJECT) apply -f k8s/pod.yaml
	kubectl -n $(PROJECT) apply -f k8s/service.yaml

k8s: k8s-clear k8s-build k8s-deploy

k8s-client-update: export DL_SKIP_SSL_VERIFICATION=1
k8s-client-update: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-update: client-update

k8s-client-get: export DL_SKIP_SSL_VERIFICATION=1
k8s-client-get: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-get: client-get

k8s-client-rebuild: DL_SKIP_SSL_VERIFICATION=1
k8s-client-rebuild: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-client-rebuild: client-rebuild

k8s-health: GRPC_SERVER = $(shell kubectl -n $(PROJECT) get service server -o custom-columns=IP:.spec.clusterIP --no-headers):$(GRPC_PORT)
k8s-health:
	grpc_health_probe -addr $(GRPC_SERVER) -tls -tls-no-verify
	grpc_health_probe -addr $(GRPC_SERVER) -tls -tls-no-verify -service $(SERVICE)

upload-container-image: release
ifndef version
	$(error version variable must be set)
else
	docker build -t gcr.io/gadget-core-production/dateilager:$(version) -t gcr.io/gadget-core-production/dateilager:latest .
	docker push gcr.io/gadget-core-production/dateilager:$(version)
	docker push gcr.io/gadget-core-production/dateilager:latest
endif

define load-test
	ghz --cert=dev/server.crt --key=dev/server.key \
		--proto internal/pb/fs.proto --call "pb.Fs.$(1)" \
		--total $(3) --concurrency $(4) --rps $(if $5,$5,0) \
		--data-file "scripts/load-tests/$(2)" \
		--metadata '{"authorization": "Bearer $(DEV_TOKEN_ADMIN)"}' \
		localhost:$(GRPC_PORT)
endef

load-test-new:
	$(call load-test,NewProject,new.json,100,1)

load-test-get:
	$(call load-test,Get,get_all.json,100000,50,5000)

load-test-update:
	$(call load-test,Update,update_increment.json,10000,1)
