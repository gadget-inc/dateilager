FROM --platform=linux/amd64 registry.fedoraproject.org/fedora-minimal:37

RUN microdnf install -y curl findutils gzip iputils less postgresql procps shadow-utils tar time which \
    && microdnf clean all

RUN GRPC_HEALTH_PROBE_VERSION=v0.4.15 \
    && curl -Lfso /bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 \
    && chmod +x /bin/grpc_health_probe

RUN GO_MIGRATE_VERSION=v4.15.2 \
    && curl -Lfso /tmp/migrate.tar.gz https://github.com/golang-migrate/migrate/releases/download/${GO_MIGRATE_VERSION}/migrate.linux-amd64.tar.gz \
    && tar -xzf /tmp/migrate.tar.gz -C /bin \
    && chmod +x /bin/migrate

RUN useradd -ms /bin/bash main
USER main
WORKDIR /home/main

RUN mkdir -p /home/main/secrets
VOLUME /home/main/secrets/tls
VOLUME /home/main/secrets/paseto

COPY release/server_linux_amd64 server
COPY migrations migrations
COPY entrypoint.sh entrypoint.sh

ENTRYPOINT ["./entrypoint.sh"]
