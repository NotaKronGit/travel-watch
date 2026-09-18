# Run the compiler natively and cross-compile for the requested image platform.
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY api ./api
COPY services/collector ./services/collector
COPY services/search ./services/search
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/search ./services/search/cmd/search && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/collector ./services/collector/cmd/collector

# Fli requires Python and glibc-compatible curl wheels. Go binaries remain static.
FROM python:3.12-slim-bookworm
COPY services/collector/tools/fli/requirements.txt /tmp/fli-requirements.txt
RUN pip install --no-cache-dir -r /tmp/fli-requirements.txt
WORKDIR /app
COPY --from=build /out/search /usr/local/bin/search
COPY services/search/config.yaml /app/config.yaml
COPY --from=build /out/collector /usr/local/bin/collector
COPY services/collector/config.yaml /app/collector.yaml
COPY services/collector/tools/fli/bridge.py /app/fli/bridge.py
ENV COLLECTOR_FLI_PYTHON=/usr/local/bin/python COLLECTOR_FLI_BRIDGE=/app/fli/bridge.py
ENV SEARCH_PLANNER_COLLECTOR_BINARY=/usr/local/bin/collector COLLECTOR_CONFIG=/app/collector.yaml
USER 10001:10001
ENTRYPOINT ["search"]
CMD ["consume"]
