FROM nixos/nix:2.22.0 AS build-stage
ARG TARGETARCH

RUN tee -a /etc/nix/nix.conf <<EOF
experimental-features = nix-command flakes
filter-syscalls = false
EOF

WORKDIR /app

COPY flake.nix flake.lock  ./
COPY development ./development

# Setup the nix environment in an early layer
RUN nix develop --command "true"

# Create a custom shell script that activates nix develop for any RUN command
RUN echo '#!/usr/bin/env bash' > /bin/nix-env-shell \
    && echo 'exec nix develop --command bash -c "$@"' >> /bin/nix-env-shell \
    && chmod +x /bin/nix-env-shell

SHELL ["/bin/nix-env-shell"]

# copy the go modules and download em
COPY go.mod go.sum ./
RUN go mod download

# copy everything else and build the project
COPY . ./
RUN make release/server_linux_$TARGETARCH release/cached_linux_$TARGETARCH

FROM buildpack-deps:bullseye AS build-release-stage
ARG TARGETARCH

RUN apt-get update && \
    apt-get install -y curl findutils gzip less lvm2 net-tools postgresql procps tar time && \
    rm -rf /var/cache/apt/archives /var/lib/apt/lists/*

RUN GRPC_HEALTH_PROBE_VERSION=v0.4.23 \
    && curl -Lfso /bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-${TARGETARCH} \
    && chmod +x /bin/grpc_health_probe

RUN GO_MIGRATE_VERSION=v4.17.1 \
    && curl -Lfso /tmp/migrate.tar.gz https://github.com/golang-migrate/migrate/releases/download/${GO_MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz \
    && tar -xzf /tmp/migrate.tar.gz -C /bin \
    && chmod +x /bin/migrate

RUN useradd -ms /bin/bash main
USER main
WORKDIR /home/main

RUN mkdir -p /home/main/secrets
VOLUME /home/main/secrets/tls
VOLUME /home/main/secrets/paseto

COPY --from=build-stage /app/release/cached_linux_${TARGETARCH} cached
COPY --from=build-stage /app/release/server_linux_${TARGETARCH} server

COPY migrations migrations
COPY entrypoint.sh entrypoint.sh

# smoke test -- ensure the commands can run
RUN ./server --help
RUN ./cached --help

ENTRYPOINT ["./entrypoint.sh"]
