# Bedrock Proxy - AWS Bedrock to Anthropic API Adapter

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![AWS Bedrock](https://img.shields.io/badge/AWS-Bedrock-FF9900?style=flat&logo=amazon-aws)](https://aws.amazon.com/bedrock/)

Un proxy HTTP de alto rendimiento que traduce requests de la API de Anthropic al formato de AWS Bedrock, permitiendo usar herramientas como Cline con modelos Claude en AWS Bedrock. Incluye autenticación JWT, control de cuotas, métricas detalladas y un sistema de logging estructurado JSON.

## 🎯 Características Principales

### Funcionalidades Core

- **🔄 Traducción Bidireccional de Formatos**: Convierte automáticamente entre Anthropic Messages API y AWS Bedrock Converse API
- **🌊 Streaming Inteligente con SSE**: Soporte completo para respuestas en streaming con Server-Sent Events
- **🏷️ Buffer XML Inteligente**: Sistema que previene el corte de tags XML entre chunks de streaming (100 caracteres)
- **🔐 Autenticación JWT Robusta**: Sistema de autenticación basado en tokens JWT con soporte para usuarios y equipos
- **📊 Métricas y Costos en Tiempo Real**: Tracking detallado de tokens (input, output, cache) y costos por usuario/equipo
- **⚖️ Control de Cuotas Flexible**: Límites configurables de tokens por usuario/equipo con reset automático diario
- **💾 Persistencia PostgreSQL**: Almacenamiento de métricas, usuarios, cuotas y eventos
- **🎯 Inference Profiles**: Soporte para AWS Bedrock Inference Profiles personalizados por usuario
- **🛠️ Tool Use Completo**: Traducción automática de herramientas Anthropic a formato Bedrock
- **📝 Logging JSON Estructurado**: Sistema de logging avanzado con sanitización de datos sensibles y rotación automática
- **🔍 Prompt Caching**: Soporte automático para AWS Bedrock Prompt Caching
- **🖥️ Computer Use**: Soporte para herramientas de Computer Use (beta)
- **🧠 Extended Thinking**: Soporte para razonamiento extendido con presupuesto de tokens configurable

### Características Técnicas

- **Arquitectura Modular**: Separación clara entre `cmd/` (aplicación) y `pkg/` (bibliotecas reutilizables)
- **Middleware Chain**: Sistema de middlewares para autenticación, cuotas, métricas y logging
- **Worker Asíncrono**: Procesamiento de métricas en background sin bloquear requests HTTP
- **Scheduler Integrado**: Reset automático de cuotas diarias con cron configurable
- **Health Checks**: Endpoint `/health` para monitoreo y orquestación
- **Context Propagation**: Sistema de contexto para tracking de requests y timing
- **Connection Pooling**: Pool de conexiones PostgreSQL optimizado (min: 5, max: 25)
- **Graceful Shutdown**: Cierre ordenado con finalización de workers y conexiones
- **Error Handling**: Manejo robusto de errores con logging detallado

## 🏗️ Arquitectura

```
┌─────────────┐
│    Cline    │
│  (Cliente)  │
└──────┬──────┘
       │ HTTP Request (Anthropic Format)
       │ Authorization: Bearer <JWT>
       ▼
┌──────────────────────────────────────────────────────────┐
│              Bedrock Proxy (Go)                          │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  HTTP Server (net/http)                            │ │
│  │  - Graceful Shutdown                               │ │
│  │  - Request Timeout: 5min                           │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Logging Middleware (amslog)                       │ │
│  │  - JSON Structured Logging                         │ │
│  │  - Request/Response Sanitization                   │ │
│  │  - Timing & Metrics                                │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Auth Middleware (JWT)                             │ │
│  │  - Token Validation                                │ │
│  │  - User/Team Extraction                            │ │
│  │  - Inference Profile Loading                       │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Quota Middleware                                  │ │
│  │  - Daily Token Limit Check                         │ │
│  │  - User/Team Quota Validation                      │ │
│  │  - Atomic Counter Updates                          │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Format Translator                                 │ │
│  │  - Anthropic Messages → Bedrock Converse           │ │
│  │  - Tool Definitions Translation                    │ │
│  │  - System Prompt Handling                          │ │
│  │  - Cache Control Translation                       │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  AWS Bedrock Client                                │ │
│  │  - ConverseStream API                              │ │
│  │  - Credential Management                           │ │
│  │  - Region Configuration                            │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Stream Processor                                  │ │
│  │  - Event Parsing (contentBlockStart, delta, stop) │ │
│  │  - XML Tag Buffer (100 chars)                      │ │
│  │  - SSE Formatting                                  │ │
│  │  - Metrics Capture                                 │ │
│  └────────────┬───────────────────────────────────────┘ │
│               ▼                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Metrics Worker (Async)                            │ │
│  │  - Channel-based Queue (buffer: 1000)              │ │
│  │  - Batch Processing                                │ │
│  │  - Cost Calculation                                │ │
│  │  - PostgreSQL Persistence                          │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Scheduler (Background)                            │ │
│  │  - Daily Quota Reset (00:00 UTC)                   │ │
│  │  - Cron-based Execution                            │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
                │ AWS Bedrock Converse API
                ▼
┌──────────────────────────────────────────────────────────┐
│              AWS Bedrock                                 │
│  - Claude 3.5 Sonnet v2                                  │
│  - Claude 3 Opus                                         │
│  - Claude 3 Haiku                                        │
│  - Inference Profiles                                    │
│  - Prompt Caching                                        │
└──────────────────────────────────────────────────────────┘
                │
                ▼
┌──────────────────────────────────────────────────────────┐
│              PostgreSQL Database                         │
│  - users (auth, quotas, profiles)                        │
│  - metrics (tokens, costs, timing)                       │
│  - events (audit log)                                    │
└──────────────────────────────────────────────────────────┘
```

## 📦 Estructura del Proyecto

```
proxy-anthropic-bedrock-v2/
├── cmd/
│   └── main.go                      # Punto de entrada de la aplicación
│                                    # - Inicialización de componentes
│                                    # - Setup de middlewares
│                                    # - Graceful shutdown
│
├── pkg/
│   ├── bedrock.go                   # Cliente Bedrock y lógica principal
│   │                                # - Traducción de formatos
│   │                                # - Manejo de streaming
│   │                                # - Procesamiento de eventos
│   │
│   ├── bedrock_metrics.go           # Captura de métricas de streaming
│   │                                # - Conteo de tokens
│   │                                # - Cálculo de costos
│   │                                # - Envío a worker asíncrono
│   │
│   ├── bedrock_tools.go             # Traducción de herramientas
│   │                                # - Anthropic tools → Bedrock tools
│   │                                # - Validación de schemas
│   │                                # - Computer Use support
│   │
│   ├── xml_buffer.go                # Buffer inteligente para tags XML
│   │                                # - Prevención de corte de tags
│   │                                # - Detección de tags incompletos
│   │                                # - Soporte para underscore
│   │
│   ├── config.go                    # Configuración de la aplicación
│   │                                # - Variables de entorno
│   │                                # - Validación de config
│   │                                # - Valores por defecto
│   │
│   ├── log.go                       # Sistema de logging legacy
│   │                                # (deprecated, usar amslog)
│   │
│   ├── events.go                    # Definición de eventos
│   │                                # - Tipos de eventos
│   │                                # - Estructuras de datos
│   │
│   ├── request_context.go           # Contexto y timing de requests
│   │                                # - Request ID
│   │                                # - Timing tracking
│   │                                # - User context
│   │
│   ├── amslog/                      # Sistema de logging estructurado
│   │   ├── logger.go                # Logger principal JSON
│   │   ├── config.go                # Configuración de logging
│   │   ├── context.go               # Context helpers
│   │   ├── event.go                 # Event definitions
│   │   ├── middleware.go            # HTTP middleware
│   │   ├── sanitizer.go             # Sanitización de datos sensibles
│   │   └── logger_test.go           # Tests unitarios
│   │
│   ├── auth/                        # Autenticación JWT
│   │   ├── jwt.go                   # Generación y validación de JWT
│   │   │                            # - Claims personalizados
│   │   │                            # - Expiración configurable
│   │   │                            # - Issuer/Audience validation
│   │   │
│   │   └── middleware.go            # Middleware de autenticación
│   │                                # - Extracción de token
│   │                                # - Validación de claims
│   │                                # - Context injection
│   │
│   ├── database/                    # Capa de persistencia
│   │   ├── database.go              # Conexión a PostgreSQL
│   │   │                            # - Connection pooling
│   │   │                            # - Health checks
│   │   │                            # - Retry logic
│   │   │
│   │   └── queries.go               # Queries SQL
│   │                                # - User management
│   │                                # - Metrics storage
│   │                                # - Quota operations
│   │                                # - Event logging
│   │
│   ├── metrics/                     # Sistema de métricas
│   │   ├── cost.go                  # Cálculo de costos
│   │   │                            # - Precios por modelo
│   │   │                            # - Cache pricing
│   │   │                            # - Conversión de unidades
│   │   │
│   │   └── worker.go                # Worker asíncrono de métricas
│   │                                # - Channel-based queue
│   │                                # - Batch processing
│   │                                # - Error handling
│   │
│   ├── quota/                       # Control de cuotas
│   │   └── middleware.go            # Middleware de control de cuotas
│   │                                # - Verificación de límites
│   │                                # - Actualización atómica
│   │                                # - User/Team quotas
│   │
│   └── scheduler/                   # Tareas programadas
│       └── scheduler.go             # Scheduler para reset de cuotas
│                                    # - Cron-based execution
│                                    # - Daily reset (00:00 UTC)
│                                    # - Error recovery
│
├── logs/                            # Directorio de logs
│   └── bedrock-proxy_*.json         # Logs JSON estructurados
│                                    # - Rotación automática
│                                    # - Formato: YYYY-MM-DD
│
├── .env.example                     # Ejemplo de configuración
├── .dockerignore                    # Exclusiones para Docker
├── Dockerfile                       # Imagen Docker
├── go.mod                           # Dependencias Go
├── go.sum                           # Checksums de dependencias
└── README.md                        # Este archivo
```

## 🚀 Instalación y Configuración

### Requisitos Previos

- **Go 1.21+** - Lenguaje de programación
- **PostgreSQL 13+** - Base de datos (opcional, para auth/métricas/cuotas)
- **AWS Account** - Con acceso a Bedrock y modelos Claude habilitados
- **AWS Credentials** - Configuradas localmente o via IAM roles

### Instalación

1. **Clonar el repositorio:**
```bash
git clone https://github.com/carlossarrion-wq/proxy-anthropic-bedrock-v2.git
cd proxy-anthropic-bedrock-v2
```

2. **Instalar dependencias:**
```bash
go mod download
```

3. **Configurar variables de entorno:**
```bash
cp .env.example .env
# Editar .env con tus credenciales
```

4. **Compilar:**
```bash
go build -o bedrock-proxy ./cmd
```

### Configuración Básica (Sin Base de Datos)

Variables mínimas requeridas en `.env`:

```bash
# AWS Bedrock Configuration
AWS_BEDROCK_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
AWS_BEDROCK_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
AWS_BEDROCK_REGION=us-east-1

# Default Model
AWS_BEDROCK_ANTHROPIC_DEFAULT_MODEL=anthropic.claude-3-5-sonnet-20241022-v2:0

# Server Configuration
PORT=8081
```

### Configuración Completa (Con PostgreSQL)

```bash
# ============================================
# AWS Bedrock Configuration
# ============================================
AWS_BEDROCK_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
AWS_BEDROCK_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
AWS_BEDROCK_REGION=us-east-1

# Default Model (usado si no se especifica en el request)
AWS_BEDROCK_ANTHROPIC_DEFAULT_MODEL=anthropic.claude-3-5-sonnet-20241022-v2:0

# Model Mappings (opcional)
# Mapea nombres cortos de Anthropic a IDs completos de Bedrock
AWS_BEDROCK_MODEL_MAPPINGS="claude-3-5-sonnet=anthropic.claude-3-5-sonnet-20241022-v2:0,claude-3-opus=anthropic.claude-3-opus-20240229-v1:0"

# ============================================
# PostgreSQL Configuration
# ============================================
DB_HOST=localhost
DB_PORT=5432
DB_NAME=bedrock_proxy
DB_USER=postgres
DB_PASSWORD=tu_password_seguro
DB_SSLMODE=require

# Connection Pool Settings
DB_MAX_CONNS=25          # Máximo de conexiones concurrentes
DB_MIN_CONNS=5           # Mínimo de conexiones en el pool

# ============================================
# JWT Authentication
# ============================================
JWT_SECRET_KEY=tu_secret_key_muy_seguro_y_largo_minimo_32_caracteres
JWT_ISSUER=bedrock-proxy
JWT_AUDIENCE=bedrock-api

# ============================================
# Advanced Features
# ============================================
# Maximum tokens per response
AWS_BEDROCK_MAX_TOKENS=8192

# Enable debug logging
AWS_BEDROCK_DEBUG=false

# Enable Computer Use (beta)
AWS_BEDROCK_ENABLE_COMPUTER_USE=false

# Enable Extended Thinking
AWS_BEDROCK_ENABLE_OUTPUT_REASON=false
AWS_BEDROCK_REASON_BUDGET_TOKENS=2048

# ============================================
# Server Configuration
# ============================================
PORT=8081

# ============================================
# Logging Configuration
# ============================================
LOG_LEVEL=info           # debug, info, warn, error
LOG_FORMAT=json          # json, text
LOG_OUTPUT=file          # file, stdout, both
LOG_FILE_PATH=./logs/bedrock-proxy.json
```

### Schema de Base de Datos

Si usas PostgreSQL, necesitas crear las siguientes tablas:

```sql
-- Tabla de usuarios
CREATE TABLE users (
    user_id VARCHAR(255) PRIMARY KEY,
    team VARCHAR(255),
    daily_token_limit BIGINT DEFAULT 1000000,
    tokens_used_today BIGINT DEFAULT 0,
    last_reset_date DATE DEFAULT CURRENT_DATE,
    default_inference_profile TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tabla de métricas
CREATE TABLE metrics (
    id SERIAL PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    team VARCHAR(255),
    model VARCHAR(255) NOT NULL,
    input_tokens BIGINT DEFAULT 0,
    output_tokens BIGINT DEFAULT 0,
    cache_read_tokens BIGINT DEFAULT 0,
    cache_write_tokens BIGINT DEFAULT 0,
    total_cost DECIMAL(10, 6) DEFAULT 0,
    request_duration_ms BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(user_id)
);

-- Tabla de eventos (audit log)
CREATE TABLE events (
    id SERIAL PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL,
    user_id VARCHAR(255),
    team VARCHAR(255),
    details JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Índices para optimizar queries
CREATE INDEX idx_metrics_user_id ON metrics(user_id);
CREATE INDEX idx_metrics_created_at ON metrics(created_at);
CREATE INDEX idx_events_user_id ON events(user_id);
CREATE INDEX idx_events_created_at ON events(created_at);
```

## 🎮 Uso

### Iniciar el Proxy

```bash
# Opción 1: Directamente con el binario
./bedrock-proxy

# Opción 2: Con go run
go run ./cmd/main.go

# Opción 3: Con variables de entorno específicas
PORT=8081 AWS_BEDROCK_DEBUG=true ./bedrock-proxy

# Opción 4: Con Docker
docker build -t bedrock-proxy .
docker run -p 8081:8081 --env-file .env bedrock-proxy
```

### Configurar Cline

En la configuración de Cline, usar:

**API Provider:** OpenAI Compatible  
**API Endpoint:** `http://localhost:8081/v1/messages`  
**API Key:** Tu token JWT (si la autenticación está habilitada)

O si está en EC2/servidor remoto:
```
API Endpoint: http://tu-ip-ec2:8081/v1/messages
```

### Endpoints Disponibles

#### POST `/v1/messages`
Endpoint principal compatible con Anthropic Messages API.

**Headers:**
```
Content-Type: application/json
Authorization: Bearer <jwt_token>  (opcional, si auth está habilitada)
```

**Request Body (ejemplo):**
```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 4096,
  "messages": [
    {
      "role": "user",
      "content": "Explica qué es un proxy HTTP"
    }
  ],
  "stream": true
}
```

**Response (streaming):**
```
event: message_start
data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant"}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Un proxy HTTP"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" es un servidor..."}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":150}}

event: message_stop
data: {"type":"message_stop"}
```

#### GET `/health`
Health check del servicio.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2026-02-10T18:30:00Z",
  "database": "connected",
  "version": "1.1.0"
}
```

## 🔐 Autenticación JWT

### Arquitectura de Autenticación

El proxy implementa un sistema de autenticación JWT robusto con las siguientes características:

- **Tokens firmados con HS256** (HMAC-SHA256)
- **Claims personalizados** para user_id, team, inference_profile
- **Validación de issuer y audience**
- **Expiración configurable** (default: 24 horas)
- **Extracción automática** del header `Authorization: Bearer <token>`

### Estructura de Claims

```go
type UserClaims struct {
    UserID                  string `json:"user_id"`
    Team                    string `json:"team"`
    DefaultInferenceProfile string `json:"default_inference_profile,omitempty"`
    jwt.RegisteredClaims
}
```

### Generar Token JWT

**Opción 1: Programáticamente en Go**

```go
package main

import (
    "fmt"
    "time"
    "bedrock-proxy-test/pkg/auth"
)

func main() {
    token, err := auth.GenerateToken(auth.UserClaims{
        UserID: "user123",
        Team:   "team-alpha",
        DefaultInferenceProfile: "arn:aws:bedrock:us-east-1:123456789:inference-profile/abc123",
    })
    
    if err != nil {
        panic(err)
    }
    
    fmt.Println("Token:", token)
}
```

**Opción 2: Con herramienta CLI (jwt-cli)**

```bash
# Instalar jwt-cli
cargo install jwt-cli

# Generar token
jwt encode \
  --secret "tu_secret_key_muy_seguro" \
  --exp "+24h" \
  --iss "bedrock-proxy" \
  --aud "bedrock-api" \
  '{"user_id":"user123","team":"team-alpha"}'
```

### Usar Token en Requests

```bash
curl -X POST http://localhost:8081/v1/messages \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Validación de Tokens

El middleware de autenticación valida automáticamente:

1. **Formato del token** (Bearer scheme)
2. **Firma del token** (HMAC-SHA256)
3. **Expiración** (exp claim)
4. **Issuer** (iss claim)
5. **Audience** (aud claim)
6. **Claims requeridos** (user_id)

Si la validación falla, retorna `401 Unauthorized`.

## 🏷️ Buffer XML - Característica Destacada

### Problema que Resuelve

Cuando AWS Bedrock envía respuestas en streaming, los chunks pueden cortar tags XML en medio, causando que herramientas como Cline no detecten correctamente las herramientas:

```
❌ Problema:
Chunk 1: "Create file <write_fi"  → Tag cortado
Chunk 2: "le>..."                  → Cline no detecta <write_file>
```

### Solución Implementada

El buffer XML inteligente detecta y retiene tags incompletos hasta que se completan:

```
✅ Solución:
Chunk 1: "Create file <write_fi"  → Buffer retiene: "<write_fi"
                                   → Envía: "Create file "
Chunk 2: "le>..."                 → Buffer completa: "<write_file>"
                                   → Envía: "<write_file>..."
```

### Características del Buffer

- **Tamaño:** 100 caracteres (suficiente para tags largos como `<thinking>`)
- **Detección inteligente:** Solo retiene texto que parece un tag XML (`<[a-zA-Z_]`)
- **Soporte underscore:** Tags como `<write_file>`, `<read_file>`, `<ask_followup_question>`
- **Chunks pequeños:** Funciona incluso con streaming letra por letra
- **Performance:** < 1ms de latencia adicional por chunk
- **Sin falsos positivos:** No retiene `<` en contenido normal (ej: "x < 5")