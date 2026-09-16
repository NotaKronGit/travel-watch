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

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/search /usr/local/bin/search
COPY services/search/config.yaml /app/config.yaml
COPY --from=build /out/collector /usr/local/bin/collector
COPY services/collector/config.yaml /app/collector.yaml
ENV SEARCH_PLANNER_COLLECTOR_BINARY=/usr/local/bin/collector COLLECTOR_CONFIG=/app/collector.yaml
USER 10001:10001
ENTRYPOINT ["search"]
CMD ["consume"]
