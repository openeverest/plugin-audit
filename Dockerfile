# ──────────────────────────────────────────────────────────────────
# Stage 1 — Build the Go backend.
# Expects backend/dist/main.js to be pre-built (npm run build) and
# ──────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS backend-builder

WORKDIR /app

COPY backend/go.mod backend/go.sum* ./
COPY src/audit-icon.png ./dist/icon.png
RUN go mod download

COPY backend/ ./

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
