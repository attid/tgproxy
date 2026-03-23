FROM golang:1.22.2-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/tgproxy .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/tgproxy /tgproxy

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/tgproxy", "healthcheck"]

ENTRYPOINT ["/tgproxy"]
