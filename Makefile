PROJECT := dateilager
VERSION := $(shell git describe --tags --abbrev=0)
BUILD_FLAGS := -ldflags="-s -w -X github.com/gadget-inc/dateilager/pkg/version.Version=$(VERSION)"

DB_HOST ?= 127.0.0.1
DB_USER ?= postgres
DB_PASS ?= password
DB_URI := postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):5432/dl

GRPC_PORT ?= 5051
GRPC_SERVER ?= localhost:$(GRPC_PORT)

DEV_TOKEN_ADMIN ?= v2.public.eyJzdWIiOiJhZG1pbiIsImlhdCI6IjIwMjEtMTAtMTVUMTE6MjA6MDAuMDM0WiJ9WtEey8KfQQRy21xoHq1C5KQatEevk8RxS47k4bRfMwVCPHumZmVuk6ADcfDHTmSnMtEGfFXdxnYOhRP6Clb_Dw
DEV_TOKEN_PROJECT_1 ?= v2.public.eyJzdWIiOiIxIiwiaWF0IjoiMjAyMS0xMC0xNVQxMToyMDowMC4wMzVaIn2MQ14RfIGpoEycCuvRu9J3CZp6PppUXf5l5w8uKKydN3C31z6f6GgOEPNcnwODqBnX7Pjarpz4i2uzWEqLgQYD

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
INTERNAL_GO_FILES := $(shell find internal/ -type f -name '*.go')
PROTO_FILES := $(shell find internal/pb/ -type f -name '*.proto')

MIGRATE_DIR := ./migrations
SERVICE := $(PROJECT).server

.PHONY: install migrate migrate-create clean build lint release
.PHONY: test test-one test-fuzz test-js lint-js build-js
.PHONY: reset-db setup-local server server-profile install-js
.PHONY: client-update client-large-update client-get client-rebuild client-pack
.PHONY: client-gc-contents client-gc-project client-gc-random-projects
.PHONY: health upload-container-image gen-docs
.PHONY: load-test-new load-test-get load-test-update

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
	go install github.com/grpc-ecosystem/grpc-health-probe@v0.4
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.15
	go install github.com/bojand/ghz/cmd/ghz@v0.110.0
	go install github.com/gadget-inc/fsdiff/cmd/fsdiff@v0.4
	go install github.com/stamblerre/gocode@latest
	go install golang.org/x/tools/cmd/goimports@latest

migrate:
	migrate -database $(DB_URI)?sslmode=disable -path $(MIGRATE_DIR) up

migrate-create:
ifndef name
	$(error name variable must be set)
else
	mkdir -p $(MIGRATE_DIR)
	migrate create -ext sql -dir $(MIGRATE_DIR) -seq $(name)
endif

clean:
	rm -f bin/*
	rm -f release/*
	rm -f internal/pb/*.pb.go
	rm -rf js/dist
	rm -rf js/node_modules
	rm -rf js/src/pb
	rm -rf input

internal/pb/%.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go_out=. --go_opt=paths=source_relative $^

internal/pb/%_grpc.pb.go: internal/pb/%.proto
	protoc --experimental_allow_proto3_optional --go-grpc_out=. --go-grpc_opt=paths=source_relative $^

bin/%: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $@ $<

development/server.key:
	mkcert -cert-file development/server.crt -key-file development/server.key localhost

development/server.crt: development/server.key

build: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go bin/server bin/client development/server.crt

lint:
	golangci-lint run

release/%_linux_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $@ $<

release/%_macos_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o $@ $<

release/%_macos_arm64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o $@ $<

release/migrations.tar.gz: migrations/*
	tar -zcf $@ migrations

release: build
release: release/server_linux_amd64 release/server_macos_amd64 release/server_macos_arm64
release: release/client_linux_amd64 release/client_macos_amd64 release/client_macos_arm64
release: release/migrations.tar.gz

test: export DB_URI = postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):5432/dl_tests
test: migrate
	cd test && go test

test-one: export DB_URI = postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):5432/dl_tests
test-one: migrate
ifndef name
	$(error name variable must be set)
else
	cd test && go test -run $(name)
endif

test-fuzz: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
test-fuzz: export DL_SKIP_SSL_VERIFICATION=1
test-fuzz: reset-db
	go run cmd/fuzz-test/main.go --server $(GRPC_SERVER) --iterations 1000 --projects 5

reset-db: migrate
	psql $(DB_URI) -c "truncate dl.objects; truncate dl.contents; truncate dl.projects; truncate dl.cache_versions;"

setup-local: reset-db
	psql $(DB_URI) -c "insert into dl.projects (id, latest_version, pack_patterns) values (1, 0, '{\"node_modules/.*/\"}');"

server: export DL_ENV=dev
server: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go
	go run cmd/server/main.go --dburi $(DB_URI) --port $(GRPC_PORT)

server-profile: export DL_ENV=dev
server-profile: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go
	go run cmd/server/main.go --dburi $(DB_URI) --port $(GRPC_PORT) --profile cpu.prof --log-level info

client-update: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-update: export DL_SKIP_SSL_VERIFICATION=1
client-update:
	development/scripts/simple_input.sh 1
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1 --dir input/simple
	development/scripts/simple_input.sh 2
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1 --dir input/simple
	development/scripts/simple_input.sh 3
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1 --dir input/simple

client-large-update: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-large-update: export DL_SKIP_SSL_VERIFICATION=1
client-large-update:
	development/scripts/complex_input.sh 1
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1  --dir input/complex
	development/scripts/complex_input.sh 2
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1 --dir input/complex
	development/scripts/complex_input.sh 3
	go run cmd/client/main.go update --server $(GRPC_SERVER) --project 1 --dir input/complex

client-get: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-get: export DL_SKIP_SSL_VERIFICATION=1
client-get:
ifndef to_version
	go run cmd/client/main.go get --server $(GRPC_SERVER) --project 1 --prefix "$(prefix)"
else
	go run cmd/client/main.go get --server $(GRPC_SERVER) --project 1 --to $(to_version) --prefix "$(prefix)"
endif

client-rebuild: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-rebuild: export DL_SKIP_SSL_VERIFICATION=1
client-rebuild:
ifndef to_version
	go run cmd/client/main.go rebuild --server $(GRPC_SERVER) --project 1 --prefix "$(prefix)" --dir $(dir)
else
	go run cmd/client/main.go rebuild --server $(GRPC_SERVER) --project 1 --to $(to_version) --prefix "$(prefix)" --dir $(dir)
endif

client-gc-contents: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-contents: export DL_SKIP_SSL_VERIFICATION=1
client-gc-contents:
	go run cmd/client/main.go gc --server $(GRPC_SERVER) --mode contents --sample 25

client-gc-project: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-project: export DL_SKIP_SSL_VERIFICATION=1
client-gc-project:
	go run cmd/client/main.go gc --server $(GRPC_SERVER) --mode project --project 1 --keep 1

client-gc-random-projects: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-random-projects: export DL_SKIP_SSL_VERIFICATION=1
client-gc-random-projects:
	go run cmd/client/main.go gc --server $(GRPC_SERVER) --mode random-projects --sample 25 --keep 1

health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)

upload-container-image: release
ifndef version
	$(error version variable must be set)
else
	docker build -t gcr.io/gadget-core-production/dateilager:$(version) -t gcr.io/gadget-core-production/dateilager:latest .
	docker push gcr.io/gadget-core-production/dateilager:$(version)
	docker push gcr.io/gadget-core-production/dateilager:latest
endif

gen-docs:
	go run cmd/gen-docs/main.go

define load-test
	ghz --cert=development/server.crt --key=development/server.key \
		--proto internal/pb/fs.proto --call "pb.Fs.$(1)" \
		--total $(3) --concurrency $(4) --rps $(if $5,$5,0) \
		--data-file "development/scripts/load-tests/$(2)" \
		--metadata '{"authorization": "Bearer $(DEV_TOKEN_ADMIN)"}' \
		localhost:$(GRPC_PORT)
endef

load-test-new: reset-db
	$(call load-test,NewProject,new.json,100,1)

load-test-update:
	$(call load-test,Update,update.json,10000,20)

load-test-update-large:
	$(call load-test,Update,update-large.json,10000,20)

load-test-get:
	$(call load-test,Get,get.json,100000,40,5000)

load-test-get-compress:
	$(call load-test,GetCompress,get-compress.json,100000,40,5000)

# JS

js/node_modules:
ifeq ($(CI),true)
	cd js && npm ci
else
	cd js && npm install
endif

js/src/pb: $(PROTO_FILES)
	cd js && mkdir -p ./src/pb && npx protoc --experimental_allow_proto3_optional --ts_out ./src/pb --ts_opt long_type_bigint,ts_nocheck,eslint_disable,add_pb_suffix --proto_path ../internal/pb/ ../$^

js/dist: js/node_modules js/src/pb
	cd js && npm run build

test-js: js/node_modules js/src/pb build migrate
	cd js && npm run test

lint-js: js/node_modules js/src/pb
	cd js && npm run lint

install-js: js/node_modules

build-js: js/dist
