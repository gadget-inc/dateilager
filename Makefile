PROJECT := dateilager
VERSION := $(shell git describe --tags --abbrev=0)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_FLAGS := -ldflags="-s -w -X github.com/gadget-inc/dateilager/pkg/version.Version=$(VERSION)"

DB_HOST ?= 127.0.0.1
DB_USER ?= postgres
DB_PASS ?= password
DB_URI := postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):5432/dl

GRPC_HOST ?= localhost
GRPC_PORT ?= 5051

CACHED_SOCKET ?= unix:///tmp/csi.sock

DEV_TOKEN_ADMIN ?= v2.public.eyJzdWIiOiJhZG1pbiJ9yt40HNkcyOUtDeFa_WPS6vi0WiE4zWngDGJLh17TuYvssTudCbOdQEkVDRD-mSNTXLgSRDXUkO-AaEr4ZLO4BQ
DEV_TOKEN_PROJECT_1 ?= v2.public.eyJzdWIiOiIxIn2jV7FOdEXafKDtAnVyDgI4fmIbqU7C1iuhKiL0lDnG1Z5-j6_ObNDd75sZvLZ159-X98_mP4qvwzui0w8pjt8F
DEV_SHARED_READER_TOKEN ?= v2.public.eyJzdWIiOiJzaGFyZWQtcmVhZGVyIn1CxWdB02s9el0Wt7qReARZ-7JtIb4Zj3D4Oiji1yXHqj0orkpbcVlswVUiekECJC16d1NrHwD2FWSwRORZn8gK

PKG_GO_FILES := $(shell find pkg/ -type f -name '*.go')
INTERNAL_GO_FILES := $(shell find internal/ -type f -name '*.go')
PROTO_FILES := $(shell find internal/pb/ -type f -name '*.proto')

MIGRATE_DIR := ./migrations
SERVICE := $(PROJECT).server
BENCH_PROFILE ?= ""
KUBE_CONTEXT ?= orbstack

.PHONY: migrate migrate-create clean build lint release prerelease
.PHONY: test test-one test-fuzz test-js lint-js install-js build-js
.PHONY: reset-db setup-local build-cache-version server server-profile cached
.PHONY: client-update client-large-update client-get client-rebuild client-rebuild-with-cache
.PHONY: client-getcache client-gc-contents client-gc-project client-gc-random-projects
.PHONY: cachedclient-probe cachedclient-populate cachedclient-stats
.PHONY: health upload-container-image upload-prerelease-container-image run-container gen-docs
.PHONY: load-test-new load-test-update load-test-update-large load-test-get load-test-get-compress
.PHONY: k8s k8s/start k8s/stop k8s/delete k8s/reset k8s/deploy
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

build: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go internal/pb/cache.pb.go internal/pb/cache_grpc.pb.go bin/server bin/client bin/cached development/server.crt

lint:
	golangci-lint run

release/%_linux_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $@ $<

release/%_linux_arm64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o $@ $<

release/%_macos_amd64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o $@ $<

release/%_macos_arm64: cmd/%/main.go $(PKG_GO_FILES) $(INTERNAL_GO_FILES) go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o $@ $<

release/migrations.tar.gz: migrations/*
	tar -zcf $@ migrations

release: build
release: release/server_linux_amd64 release/server_macos_amd64 release/server_macos_arm64 release/server_linux_arm64
release: release/client_linux_amd64 release/client_macos_amd64 release/client_macos_arm64 release/client_linux_arm64
release: release/cached_linux_amd64 release/cached_macos_amd64 release/cached_macos_arm64 release/cached_linux_arm64
release: release/migrations.tar.gz

prerelease: build
prerelease: build-js
prerelease:
ifndef tag
	$(error tag variable must be set)
else
	cd js && npx ts-node dateilager-prerelease.ts -t "$(tag)"
endif

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

bench: export DB_URI = postgres://$(DB_USER):$(DB_PASS)@$(DB_HOST):5432/dl_tests
bench: migrate
	cd test && go test -bench . -run=^# $(BENCH_PROFILE)

bench/cpu: export BENCH_PROFILE = -cpuprofile cpu.pprof
bench/cpu: bench

test-fuzz: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
test-fuzz: export DL_SKIP_SSL_VERIFICATION=1
test-fuzz: reset-db
	go run cmd/fuzz-test/main.go --host $(GRPC_HOST) --iterations 1000 --projects 5

reset-db: migrate
	psql $(DB_URI) -c "truncate dl.objects; truncate dl.contents; truncate dl.projects; truncate dl.cache_versions;"

setup-local: reset-db
	psql $(DB_URI) -c "insert into dl.projects (id, latest_version, pack_patterns) values (1, 0, '{\"node_modules/.*/\"}');"

build-cache-version:
	psql $(DB_URI) -c "with impactful_packed_objects as (select hash, count(*) as count from dl.objects where packed = true and stop_version is null group by hash order by count desc limit 20) insert into dl.cache_versions (hashes) select coalesce(array_agg(hash), '{}') from impactful_packed_objects;"

server: export DL_ENV=dev
server: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go development/server.crt
	go run cmd/server/main.go --dburi $(DB_URI) --port $(GRPC_PORT)

server-profile: export DL_ENV=dev
server-profile: internal/pb/fs.pb.go internal/pb/fs_grpc.pb.go development/server.crt
	go run cmd/server/main.go --dburi $(DB_URI) --port $(GRPC_PORT) --profile cpu.prof --log-level info

cached: export DL_ENV=dev
cached: export DL_TOKEN=$(DEV_SHARED_READER_TOKEN)
cached: internal/pb/cache.pb.go internal/pb/cache_grpc.pb.go
	go run cmd/cached/main.go --upstream-host $(GRPC_HOST) --upstream-port $(GRPC_PORT) --csi-socket $(CACHED_SOCKET) --staging-path tmp/cache-stage

client-update: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-update: export DL_SKIP_SSL_VERIFICATION=1
client-update:
	development/scripts/simple_input.sh 1
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1 --dir input/simple
	development/scripts/simple_input.sh 2
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1 --dir input/simple
	development/scripts/simple_input.sh 3
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1 --dir input/simple

client-large-update: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-large-update: export DL_SKIP_SSL_VERIFICATION=1
client-large-update:
	development/scripts/complex_input.sh 1
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1  --dir input/complex
	development/scripts/complex_input.sh 2
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1 --dir input/complex
	development/scripts/complex_input.sh 3
	go run cmd/client/main.go update --host $(GRPC_HOST) --project 1 --dir input/complex

client-get: export DL_TOKEN=$(DEV_TOKEN_PROJECT_1)
client-get: export DL_SKIP_SSL_VERIFICATION=1
client-get:
ifndef to_version
	go run cmd/client/main.go get --host $(GRPC_HOST) --project 1 --prefix "$(prefix)"
else
	go run cmd/client/main.go get --host $(GRPC_HOST) --project 1 --to $(to_version) --prefix "$(prefix)"
endif

client-rebuild: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-rebuild: export DL_SKIP_SSL_VERIFICATION=1
client-rebuild:
ifndef to_version
	go run cmd/client/main.go rebuild --host $(GRPC_HOST) --project 1 --prefix "$(prefix)" --dir $(dir)
else
	go run cmd/client/main.go rebuild --host $(GRPC_HOST) --project 1 --to $(to_version) --prefix "$(prefix)" --dir $(dir)
endif

client-rebuild-with-cache: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-rebuild-with-cache: export DL_SKIP_SSL_VERIFICATION=1
client-rebuild-with-cache:
	go run cmd/client/main.go rebuild --host $(GRPC_HOST) --project 1 --prefix "$(prefix)" --dir $(dir) --cachedir input/cache

client-getcache: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-getcache: export DL_SKIP_SSL_VERIFICATION=1
client-getcache:
	go run cmd/client/main.go getcache --host $(GRPC_HOST) --path input/cache

client-gc-contents: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-contents: export DL_SKIP_SSL_VERIFICATION=1
client-gc-contents:
	go run cmd/client/main.go gc --host $(GRPC_HOST) --mode contents --sample 25

client-gc-project: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-project: export DL_SKIP_SSL_VERIFICATION=1
client-gc-project:
	go run cmd/client/main.go gc --host $(GRPC_HOST) --mode project --project 1 --keep 1

client-gc-random-projects: export DL_TOKEN=$(DEV_TOKEN_ADMIN)
client-gc-random-projects: export DL_SKIP_SSL_VERIFICATION=1
client-gc-random-projects:
	go run cmd/client/main.go gc --host $(GRPC_HOST) --mode random-projects --sample 25 --keep 1

cachedclient-probe:
	go run cmd/cached-client/main.go probe --socket $(CACHED_SOCKET)

cachedclient-populate:
	go run cmd/cached-client/main.go populate --socket $(CACHED_SOCKET) --path input/cache

cachedclient-stats:
	go run cmd/cached-client/main.go stats --socket $(CACHED_SOCKET)

health:
	grpc-health-probe -addr $(GRPC_SERVER)
	grpc-health-probe -addr $(GRPC_SERVER) -service $(SERVICE)

upload-container-image:
ifndef version
	$(error version variable must be set)
else
	docker build --platform linux/arm64,linux/amd64 --push -t us-central1-docker.pkg.dev/gadget-core-production/core-production/dateilager:$(version) -t us-central1-docker.pkg.dev/gadget-core-production/core-production/dateilager:latest .
endif

upload-prerelease-container-image:
	docker build --platform linux/arm64,linux/amd64 --push -t us-central1-docker.pkg.dev/gadget-core-production/core-production/dateilager:pre-$(GIT_COMMIT) .

build-local-container:
	docker build --load -t dl-local:dev .

run-container: release build-local-container
	docker run --rm -it -p 127.0.0.1:$(GRPC_PORT):$(GRPC_PORT)/tcp -v ./development:/home/main/secrets/tls -v ./development:/home/main/secrets/paseto dl-local:dev $(GRPC_PORT) "postgres://$(DB_USER):$(DB_PASS)@host.docker.internal:5432" dl

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

js/src/pb: internal/pb/fs.proto
	cd js && mkdir -p ./src/pb && npx protoc --experimental_allow_proto3_optional --ts_out ./src/pb --ts_opt long_type_bigint,ts_nocheck,eslint_disable,add_pb_suffix --proto_path ../internal/pb/ ../internal/pb/fs.proto

js/dist: js/node_modules js/src/pb
	cd js && npm run build

test-js: js/node_modules js/src/pb build migrate
	cd js && npm run test

lint-js: js/node_modules js/src/pb
	cd js && npm run lint

install-js: js/node_modules

build-js: js/dist

k8s:
	which orb >/dev/null 2>&1; if [ $$? -ne 0 ]; then echo "orb not found"; exit 1; fi
k8s/start: k8s	
	orb start k8s

k8s/stop: k8s
	orb stop k8s

k8s/delete: k8s
	orb delete k8s

k8s/reset: k8s/stop k8s/delete k8s/start

k8s/deploy: k8s k8s/start
	kubectl --context=$(KUBE_CONTEXT) create namespace dateilager-local || true
	kubectl --context=$(KUBE_CONTEXT) apply -f test/k8s-local/cached-csi.yaml -n dateilager-local
	kubectl --context=$(KUBE_CONTEXT) apply -f test/k8s-local/cached-daemon.yaml -n dateilager-local

k8s/reset_namespace: k8s k8s/start
	kubectl --context=$(KUBE_CONTEXT) delete ds dateilager-csi-cached -n dateilager-local --force --grace-period=0 || true
	kubectl --context=$(KUBE_CONTEXT) delete pod busybox-csi -n dateilager-local --force --grace-period=0 || true
	kubectl --context=$(KUBE_CONTEXT) delete namespace dateilager-local || true
