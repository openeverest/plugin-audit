# ──────────────────────────────────────────────────────────────────
# Stage 1 — Build the Go backend.
# Expects dist/main.js to be pre-built (npm run build) and present
# in the Docker build context.
# ──────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS backend-builder

RUN apk --no-cache add build-base

WORKDIR /app

COPY backend/go.mod backend/go.sum* ./
RUN go mod download

COPY backend/ .
COPY dist/main.js ./dist/main.js
COPY src/icon.png ./dist/icon.png

# CGO is required by the modernc.SQLite-free driver alternative; we use
# modernc.org/sqlite (pure Go) so CGO can stay disabled.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o server .

# ──────────────────────────────────────────────────────────────────
# Stage 2 — Minimal runtime image.
# ──────────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=backend-builder /app/server /usr/local/bin/server

EXPOSE 8080

ENTRYPOINT ["server"]
