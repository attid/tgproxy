FROM golang:1.22.2-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/tgproxy .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/tgproxy /tgproxy

EXPOSE 8080

ENTRYPOINT ["/tgproxy"]
