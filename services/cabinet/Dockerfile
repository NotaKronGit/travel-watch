FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen ./gen
COPY services/cabinet ./services/cabinet
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cabinet ./services/cabinet/cmd/cabinet

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/cabinet /usr/local/bin/cabinet
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["cabinet"]
CMD ["serve"]
