FROM registry.fedoraproject.org/fedora-minimal:33

RUN microdnf install -y curl findutils iputils procps time which \
    && microdnf clean all

RUN GRPC_HEALTH_PROBE_VERSION=v0.4.2 \
    && curl -Lfso /bin/grpc_health_probe https://github.com/grpc-ecosystem/grpc-health-probe/releases/download/${GRPC_HEALTH_PROBE_VERSION}/grpc_health_probe-linux-amd64 \
    && chmod +x /bin/grpc_health_probe

RUN useradd -ms /bin/bash main
USER main
WORKDIR /home/main

COPY bin/server server

ENTRYPOINT ["./server"]
