# Dockerfile Multi-Stage para Bedrock Proxy
# Optimizado para producción con imagen mínima

# ============================================
# STAGE 1: Builder
# ============================================
FROM golang:1.24-alpine AS builder

# Instalar dependencias de compilación
RUN apk add --no-cache git ca-certificates tzdata

# Establecer directorio de trabajo
WORKDIR /build

# Copiar archivos de dependencias primero (mejor cache de Docker)
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fuente
COPY . .

# Compilar aplicación con optimizaciones
# CGO_ENABLED=0: Binario estático sin dependencias de C
# -ldflags="-w -s": Reducir tamaño del binario (eliminar símbolos de debug)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o bedrock-proxy \
    ./cmd/main.go

# ============================================
# STAGE 2: Runtime
# ============================================
FROM alpine:3.19

# Instalar certificados CA y zona horaria
RUN apk --no-cache add ca-certificates tzdata

# Crear usuario no-root para seguridad
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Establecer directorio de trabajo
WORKDIR /app

# Copiar binario compilado desde builder
COPY --from=builder /build/bedrock-proxy .

# Cambiar ownership al usuario no-root
RUN chown -R appuser:appuser /app

# Cambiar a usuario no-root
USER appuser

# Exponer puerto de la aplicación
EXPOSE 8080

# Health check para ECS
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Comando de inicio
ENTRYPOINT ["./bedrock-proxy"]