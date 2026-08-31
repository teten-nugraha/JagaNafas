FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/riskengine ./cmd/riskengine
RUN mkdir -p /out/logs

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build --chown=nonroot:nonroot /out/riskengine .
COPY --from=build --chown=nonroot:nonroot /out/logs ./logs

ENV LOG_FILE=/app/logs/app.log

EXPOSE 8080
ENTRYPOINT ["/app/riskengine"]
