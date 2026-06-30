# ──────────────────────────────────────────────────────────────────
# Stage 1 — Build the frontend bundle that gets embedded into the Go
# binary. Vite is configured (vite.config.ts) to emit to backend/dist/main.js.
# ──────────────────────────────────────────────────────────────────
FROM node:20-alpine AS frontend-builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY tsconfig.json vite.config.ts ./
COPY src/ ./src/

RUN npm run build

# ──────────────────────────────────────────────────────────────────
# Stage 2 — Build the Go backend, embedding the frontend bundle and icon.
# ──────────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS backend-builder

WORKDIR /app

COPY backend/go.mod backend/go.sum* ./
RUN go mod download

COPY backend/ ./

# Provide the assets required by //go:embed directives in backend/main.go.
COPY src/audit-icon.png ./dist/icon.png
COPY --from=frontend-builder /app/backend/dist/main.js ./dist/main.js

# We use modernc.org/sqlite (pure Go), so CGO can stay disabled.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o server .

# ──────────────────────────────────────────────────────────────────
# Stage 3 — Minimal runtime image.
# ──────────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=backend-builder /app/server /usr/local/bin/server

EXPOSE 8080

ENTRYPOINT ["server"]
