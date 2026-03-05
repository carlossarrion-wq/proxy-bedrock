# Bedrock Proxy

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![AWS Bedrock](https://img.shields.io/badge/AWS-Bedrock-FF9900?style=flat&logo=amazon-aws)](https://aws.amazon.com/bedrock/)

Proxy HTTP de alto rendimiento que adapta la API de Anthropic al formato de AWS Bedrock, permitiendo usar herramientas como Cline con modelos Claude alojados en AWS Bedrock.

## 🎯 Características Principales

### Funcionalidades Core

- **Traducción de Formatos**: Conversión automática entre Anthropic Messages API y AWS Bedrock Converse API
- **Streaming con SSE**: Soporte completo para respuestas en tiempo real con Server-Sent Events
- **Buffer XML Inteligente**: Previene el corte de tags XML entre chunks de streaming
- **Autenticación JWT**: Sistema robusto con validación de tokens y rate limiting
- **Control de Cuotas**: Límites configurables por usuario/equipo con reset automático
- **Métricas en Tiempo Real**: Tracking detallado de tokens, costos y rendimiento
- **Prompt Caching**: Soporte automático para AWS Bedrock Prompt Caching
- **Logging Estructurado**: Sistema de logs JSON con sanitización de datos sensibles

### Características Técnicas

- **Arquitectura Modular**: Separación clara entre aplicación y bibliotecas reutilizables
- **Worker Asíncrono**: Procesamiento de métricas en background sin bloquear requests
- **Scheduler Integrado**: Reset automático de cuotas diarias
- **Connection Pooling**: Pool optimizado de conexiones PostgreSQL
- **Graceful Shutdown**: Cierre ordenado con finalización de workers
- **Health Checks**: Endpoint para monitoreo y orquestación

## 🏗️ Arquitectura

### Flujo de Request

```
Cliente (Cline) → Logging Middleware → Auth Middleware → Quota Middleware 
→ Format Translator → AWS Bedrock → Stream Processor → Metrics Worker
```

### Componentes Principales

**HTTP Server**
- Puerto configurable (default: 8080)
- Timeout de 5 minutos por request
- Graceful shutdown con finalización de workers

**Middleware Chain**
- **Logging**: Registro estructurado de todas las operaciones
- **Authentication**: Validación JWT con rate limiting por IP y token
- **Quota**: Verificación de límites diarios y mensuales

**Bedrock Client**
- Traducción bidireccional de formatos
- Manejo de streaming con buffer XML
- Soporte para tools, imágenes y cache control

**Metrics System**
- Worker asíncrono con queue de 1000 elementos
- Cálculo automático de costos por modelo
- Persistencia en PostgreSQL

**Scheduler**
- Reset diario de cuotas a medianoche UTC
- Ejecución basada en cron

## 📦 Estructura del Proyecto

```
proxy-bedrock/
├── cmd/
│   └── main.go                    # Punto de entrada de la aplicación
│
├── pkg/
│   ├── bedrock.go                 # Cliente Bedrock y lógica principal
│   ├── bedrock_metrics.go         # Captura de métricas de streaming
│   ├── bedrock_tools.go           # Traducción de herramientas
│   ├── xml_buffer.go              # Buffer inteligente para tags XML
│   ├── config.go                  # Configuración de la aplicación
│   ├── request_context.go         # Contexto y timing de requests
│   ├── events.go                  # Definición de eventos
│   │
│   ├── amslog/                    # Sistema de logging estructurado
│   │   ├── logger.go              # Logger principal JSON
│   │   ├── config.go              # Configuración de logging
│   │   ├── middleware.go          # HTTP middleware
│   │   └── sanitizer.go           # Sanitización de datos sensibles
│   │
│   ├── auth/                      # Autenticación JWT
│   │   ├── jwt.go                 # Generación y validación de JWT
│   │   ├── middleware.go          # Middleware de autenticación
│   │   ├── rate_limiter.go        # Rate limiting
│   │   └── usage_tracking.go      # Tracking de uso
│   │
│   ├── database/                  # Capa de persistencia
│   │   ├── database.go            # Conexión a PostgreSQL
│   │   ├── queries.go             # Queries SQL
│   │   └── quota_queries.go       # Queries de cuotas
│   │
│   ├── metrics/                   # Sistema de métricas
│   │   ├── cost.go                # Cálculo de costos por modelo
│   │   ├── model_resolver.go     # Resolución de ARNs a modelos
│   │   └── worker.go              # Worker asíncrono
│   │
│   ├── quota/                     # Control de cuotas
│   │   └── middleware.go          # Middleware de cuotas
│   │
│   └── scheduler/                 # Tareas programadas
│       └── scheduler.go           # Reset diario de cuotas
│
├── logs/                          # Directorio de logs
├── Dockerfile                     # Imagen Docker multi-stage
├── go.mod                         # Dependencias Go
└── README.md                      # Este archivo
```

## 🚀 Instalación y Configuración

### Requisitos Previos

- Go 1.24 o superior
- PostgreSQL 13+ (opcional, para autenticación y métricas)
- AWS Account con acceso a Bedrock
- AWS Credentials configuradas

### Instalación

1. Clonar el repositorio
2. Instalar dependencias: `go mod download`
3. Configurar variables de entorno (ver `.env.example`)
4. Compilar: `go build -o bedrock-proxy ./cmd`

### Configuración Básica

Variables mínimas requeridas:

- `AWS_BEDROCK_ACCESS_KEY`: Access key de AWS
- `AWS_BEDROCK_SECRET_KEY`: Secret key de AWS
- `AWS_BEDROCK_REGION`: Región de AWS (ej: us-east-1)
- `AWS_BEDROCK_ANTHROPIC_DEFAULT_MODEL`: Modelo por defecto
- `PORT`: Puerto del servidor (default: 8081)

### Configuración Completa

Para habilitar todas las funcionalidades, configurar también:

**Base de Datos**
- `DB_SECRET_ARN`: ARN del secreto en AWS Secrets Manager (recomendado)
- O variables individuales: `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`
- `DB_SSLMODE`: Modo SSL (default: require)
- `DB_MAX_CONNS`: Conexiones máximas (default: 25)
- `DB_MIN_CONNS`: Conexiones mínimas (default: 5)

**JWT Authentication**
- `JWT_SECRET_ARN`: ARN del secreto JWT en AWS Secrets Manager (recomendado)
- O `JWT_SECRET_KEY`: Clave secreta (mínimo 32 caracteres)
- `JWT_ISSUER`: Emisor del token (default: identity-manager)
- `JWT_AUDIENCE`: Audiencia del token (default: bedrock-proxy)

**Características Avanzadas**
- `AWS_BEDROCK_MAX_TOKENS`: Tokens máximos por respuesta (default: 8192)
- `AWS_BEDROCK_FORCE_PROMPT_CACHING`: Forzar prompt caching (default: true)
- `AWS_BEDROCK_ENABLE_COMPUTER_USE`: Habilitar Computer Use (default: false)
- `AWS_BEDROCK_ENABLE_OUTPUT_REASON`: Habilitar Extended Thinking (default: false)
- `AWS_BEDROCK_DEBUG`: Modo debug (default: false)

**Logging**
- `LOG_LEVEL`: Nivel de log (debug, info, warn, error)
- `LOG_FORMAT`: Formato de log (json, text)
- `LOG_OUTPUT`: Salida de log (file, stdout, both)
- `LOG_FILE_PATH`: Ruta del archivo de log

## 🎮 Uso

### Iniciar el Proxy

**Opción 1: Binario directo**
```bash
./bedrock-proxy
```

**Opción 2: Con Go**
```bash
go run ./cmd/main.go
```

**Opción 3: Con Docker**
```bash
docker build -t bedrock-proxy .
docker run -p 8081:8081 --env-file .env bedrock-proxy
```

### Configurar Cline

En la configuración de Cline:

- **API Provider**: OpenAI Compatible
- **API Endpoint**: `http://localhost:8081/v1/messages`
- **API Key**: Tu token JWT (si la autenticación está habilitada)

### Endpoints Disponibles

**POST `/v1/messages`**
- Endpoint principal compatible con Anthropic Messages API
- Requiere header `Authorization: Bearer <jwt_token>` (si auth habilitada)
- Soporta streaming con `"stream": true`

**GET `/health`**
- Health check del servicio
- Retorna estado del servicio y base de datos

## 🔐 Autenticación JWT

### Características

- Tokens firmados con HS256 (HMAC-SHA256)
- Claims personalizados: user_id, team, inference_profile
- Validación de issuer y audience
- Expiración configurable (default: 24 horas)
- Auto-regeneración de tokens expirados

### Rate Limiting

- **Por IP**: 5 intentos fallidos por minuto
- **Por Token**: 10 intentos fallidos por minuto
- Backoff automático con header `Retry-After`

### Estructura de Claims

Los tokens JWT incluyen:
- `user_id`: Identificador del usuario
- `email`: Email del usuario
- `team`: Equipo al que pertenece
- `person`: Nombre de la persona
- `default_inference_profile`: ARN del perfil de inferencia
- `exp`: Fecha de expiración
- `iss`: Emisor del token
- `aud`: Audiencia del token

## 📊 Sistema de Métricas

### Métricas Capturadas

- **Tokens**: Input, output, cache read, cache write
- **Costos**: Cálculo automático en USD por modelo
- **Timing**: Duración por fase (sign, parse, streaming, post-process)
- **Errores**: Stack traces y mensajes de error

### Modelos Soportados

**Claude 3.5 Family**
- Sonnet v2: $3/1M input, $15/1M output
- Haiku: $1/1M input, $5/1M output

**Claude 3 Family**
- Opus: $15/1M input, $75/1M output
- Sonnet: $3/1M input, $15/1M output
- Haiku: $0.25/1M input, $1.25/1M output

**Claude Sonnet 4.5**
- Input: $3/1M tokens
- Output: $15/1M tokens
- Cache Write: $3.75/1M tokens (25% premium)
- Cache Read: $0.30/1M tokens (90% descuento)

### Prompt Caching

El proxy soporta automáticamente Prompt Caching de AWS Bedrock:

- **Cache Write**: Tokens escritos en caché (primera vez)
- **Cache Read**: Tokens leídos desde caché (subsecuentes)
- **Ahorro**: Hasta 90% de descuento en tokens cacheados

Configuración:
- `AWS_BEDROCK_FORCE_PROMPT_CACHING=true`: Añade cache points automáticamente
- `AWS_BEDROCK_FORCE_PROMPT_CACHING=false`: Respeta cache_control del cliente

## ⚖️ Control de Cuotas

### Características

- Límites diarios y mensuales por usuario/equipo
- Reset automático a medianoche UTC
- Bloqueo automático al exceder límites
- Headers de rate limit en respuestas

### Headers de Rate Limit

Todas las respuestas incluyen:
- `X-RateLimit-Limit`: Límite diario de requests
- `X-RateLimit-Remaining`: Requests restantes
- `X-RateLimit-Reset`: Timestamp del próximo reset

### Comportamiento al Exceder Cuota

Cuando se excede la cuota:
- Status code: 401 Unauthorized
- Header `Retry-After`: Segundos hasta el reset
- Mensaje de error descriptivo con información de cuota

## 📝 Sistema de Logging

### Formato de Logs

Logs estructurados en formato JSON compatible con ECS (Elastic Common Schema):

- `@timestamp`: Timestamp ISO 8601
- `log.level`: Nivel de log (DEBUG, INFO, WARN, ERROR, FATAL)
- `service.name`: Nombre del servicio
- `event.name`: Nombre del evento
- `event.outcome`: Resultado (success, failure)
- `trace.id`: ID de traza para correlación
- `request.id`: ID único del request
- `event.duration_ms`: Duración en milisegundos

### Eventos Principales

- `PROXY_REQUEST_START`: Inicio de request
- `AUTH_SUCCESS/FAILURE`: Resultado de autenticación
- `QUOTA_EXCEEDED`: Cuota excedida
- `BEDROCK_INVOKE`: Llamada a Bedrock
- `BEDROCK_STREAM_COMPLETE`: Streaming completado
- `PROXY_REQUEST_END`: Fin de request

### Sanitización

El sistema sanitiza automáticamente:
- Tokens JWT en headers
- Credenciales AWS
- Datos sensibles en payloads
- Información personal identificable

## 🏷️ Buffer XML - Característica Destacada

### Problema que Resuelve

AWS Bedrock puede enviar chunks de streaming que cortan tags XML en medio, causando que herramientas como Cline no detecten correctamente las herramientas.

### Solución

Buffer inteligente de 100 caracteres que:
- Detecta tags XML incompletos
- Retiene el texto hasta completar el tag
- Envía el contenido completo cuando el tag se cierra
- Soporta tags con underscore (ej: `<write_file>`)
- Añade latencia mínima (< 1ms por chunk)

### Configuración

Variable de entorno `XML_BUFFER_MAX_SIZE` (default: 100 caracteres)

## 🐳 Despliegue

### Docker

**Construir imagen:**
```bash
docker build -t bedrock-proxy .
```

**Ejecutar contenedor:**
```bash
docker run -p 8081:8081 --env-file .env bedrock-proxy
```

### AWS ECS

**Subir imagen a ECR:**
```bash
# Login en ECR
aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 701055077130.dkr.ecr.eu-west-1.amazonaws.com

# Tag y push
docker tag bedrock-proxy:latest 701055077130.dkr.ecr.eu-west-1.amazonaws.com/bedrock-proxy:latest
docker push 701055077130.dkr.ecr.eu-west-1.amazonaws.com/bedrock-proxy:latest
```

**Actualizar servicio:**
```bash
aws ecs update-service \
  --cluster bedrock-proxy-dev-cluster \
  --service bedrock-proxy-dev-service \
  --force-new-deployment \
  --region eu-west-1
```

### Características del Dockerfile

- Multi-stage build para imagen mínima
- Usuario no-root para seguridad
- Health check integrado
- Binario estático sin dependencias C
- Imagen final basada en Alpine Linux

## 🔧 Dependencias

### Principales

- `aws-sdk-go-v2`: Cliente AWS Bedrock
- `pgx/v5`: Driver PostgreSQL de alto rendimiento
- `golang-jwt/v5`: Manejo de JWT
- `google/uuid`: Generación de UUIDs
- `lumberjack.v2`: Rotación de logs

### Versiones

- Go: 1.24+
- PostgreSQL: 13+
- Docker: 20.10+

## 📈 Rendimiento

### Optimizaciones

- Connection pooling para PostgreSQL (5-25 conexiones)
- Worker asíncrono para métricas (no bloquea requests)
- Buffer de 1000 elementos para queue de métricas
- Graceful shutdown con timeout configurable
- Streaming eficiente con buffer XML mínimo

### Límites

- Timeout por request: 5 minutos
- Máximo de mensajes por request: 1000
- Buffer de métricas: 1000 elementos
- Conexiones PostgreSQL: 5-25 (configurable)

## 🔒 Seguridad

### Mejores Prácticas Implementadas

- JWT secret mínimo 32 caracteres (OWASP)
- Soporte para AWS Secrets Manager
- Rate limiting por IP y token
- Sanitización automática de logs
- Usuario no-root en Docker
- Validación de tokens contra base de datos
- Bloqueo automático por intentos fallidos

### Recomendaciones

- Usar AWS Secrets Manager para credenciales
- Habilitar SSL para PostgreSQL
- Configurar rate limiting apropiado
- Revisar logs regularmente
- Mantener tokens JWT con expiración corta
- Rotar credenciales periódicamente

## 📚 Recursos Adicionales

### Documentación AWS

- [AWS Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
- [Bedrock Converse API](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html)
- [Bedrock Prompt Caching](https://docs.aws.amazon.com/bedrock/latest/userguide/prompt-caching.html)

### Documentación Anthropic

- [Anthropic Messages API](https://docs.anthropic.com/claude/reference/messages_post)
- [Tool Use Guide](https://docs.anthropic.com/claude/docs/tool-use)
- [Prompt Caching](https://docs.anthropic.com/claude/docs/prompt-caching)

## 🤝 Contribución

Este proyecto sigue las mejores prácticas de Go y mantiene una arquitectura modular. Para contribuir:

1. Mantener la estructura modular existente
2. Seguir las convenciones de naming de Go
3. Añadir tests para nuevas funcionalidades
4. Actualizar documentación según cambios
5. Usar logging estructurado para nuevos eventos

## 📄 Licencia

MIT License - Ver archivo LICENSE para detalles

## 📞 Soporte

Para reportar issues o solicitar features, usar el sistema de issues del repositorio.

---

**Versión**: 1.1.0  
**Última actualización**: Marzo 2026  
**Mantenido por**: Identity Manager Team