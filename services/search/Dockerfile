# Run the compiler natively and cross-compile for the requested image platform.
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY services/search ./services/search
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/search ./services/search/cmd/search

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/search /usr/local/bin/search
COPY services/search/config.yaml /app/config.yaml
USER 10001:10001
ENTRYPOINT ["search"]
CMD ["consume"]
