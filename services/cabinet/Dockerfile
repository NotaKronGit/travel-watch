# Run the compiler natively and cross-compile for the requested image platform.
FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY api/progress ./api/progress
COPY services/cabinet ./services/cabinet
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/cabinet ./services/cabinet/cmd/cabinet

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/cabinet /usr/local/bin/cabinet
COPY services/cabinet/config.yaml /app/config.yaml
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["cabinet"]
CMD ["serve"]
