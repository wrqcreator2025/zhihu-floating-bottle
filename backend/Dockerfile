FROM golang:1.25.0-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worker ./cmd/worker && \
    go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/api /api
COPY --from=build /out/worker /worker
COPY --from=build /go/bin/migrate /migrate
COPY migrations /migrations
COPY scripts/migrate-railway.sh /migrate-railway.sh
RUN chmod 0555 /migrate-railway.sh
EXPOSE 8080
USER 65532:65532
CMD ["/api"]
