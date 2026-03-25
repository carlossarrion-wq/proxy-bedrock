# System Patterns

## System Architecture

### High-Level Design
```
[Client Request]
      ↓
[Logging Middleware] ← Request Context & Sanitization
      ↓
[Authentication Middleware] ← JWT Validation & Rate Limiting
      ↓
[Quota Middleware] ← Check Usage Quotas
      ↓
[Bedrock Proxy Handler]
      ├─ Sign Request (AWS SigV4)
      ├─ Parse & Convert Format
      ├─ Call Bedrock Converse API
      └─ Stream Response (SSE)
      ↓
[Metrics Capture] → [Background Worker] → [Database]
      ↓
[Client Response]

Background Services:
- Metrics Worker (async cost/usage tracking)
- Scheduler (daily quota reset at midnight UTC)
```

### Core Components

**1. HTTP Server** (`cmd/main.go`)
- Port configurable (default: 8080)
- Middleware chain setup
- Graceful shutdown handling
- Health check endpoint

**2. Authentication Layer** (`pkg/auth/`)
- JWT token validation (HS256)
- API key authentication
- Rate limiting (in-memory):
  - 5 failed attempts/min per IP
  - 10 failed attempts/min per token
- Usage tracking integration

**3. Quota Middleware** (`pkg/quota/`)
- Daily and monthly limit enforcement
- Automatic quota checks before processing
- Rate limit headers in responses
- User blocking on quota exceeded

**4. Bedrock Proxy** (`pkg/bedrock.go`)
- AWS SigV4 request signing
- Format translation (Anthropic ↔ Bedrock)
- Streaming with SSE
- XML buffer for tag completion
- Tool use support
- Image handling
- Prompt caching support

**5. Logging System** (`pkg/amslog/`)
- Structured JSON logging
- ECS-compatible format
- Request sanitization
- Context propagation (trace ID, request ID)
- Multiple log levels (DEBUG, INFO, WARN, ERROR, FATAL)

**6. Metrics & Cost Tracking** (`pkg/metrics/`)
- Real-time cost calculation
- Token usage tracking
- Model-specific pricing
- Background worker (1000-element queue)
- Async database writes

**7. Database Layer** (`pkg/database/`)
- PostgreSQL connection pooling
- User and API key management
- Usage tracking tables
- Quota queries
- AWS Secrets Manager integration

**8. Scheduler** (`pkg/scheduler/`)
- Cron-based task execution
- Daily quota reset (midnight UTC)
- Graceful shutdown support

## Key Technical Decisions

### Authentication & Security
- **JWT Tokens**: HS256 signing, configurable expiration (default: 24h)
- **API Keys**: Database-stored with hashed secrets
- **Rate Limiting**: In-memory per-key limits (resets on restart)
- **Request Sanitization**: Prevents log injection attacks
- **CORS**: Configurable for web client support

### Database Strategy
- **PostgreSQL**: Production-ready, supports high concurrency
- **Connection Pooling**: 5-25 connections (configurable)
- **Prepared Statements**: Prevents SQL injection
- **Transaction Support**: Ensures data consistency
- **Graceful Shutdown**: Proper connection cleanup

### Streaming Architecture
- **SSE (Server-Sent Events)**: Standard streaming protocol
- **XML Buffering**: Critical for Claude response streaming
- **Context Cancellation**: Proper cleanup on client disconnect
- **Chunked Transfer**: Efficient memory usage
- **Selective Buffering**: Only send message_start after receiving real token counts

### Error Handling
- **Consistent Responses**: Standard error format across all endpoints
- **HTTP Status Codes**: Proper semantic status codes (401, 429, 500, etc.)
- **Error Logging**: All errors logged with context
- **Graceful Degradation**: Continues operation on non-critical errors

## Design Patterns in Use

### 1. Middleware Pattern
- **Chain of Responsibility**: Sequential request processing
- **Separation of Concerns**: Each middleware handles one aspect
- **Composability**: Easy to add/remove middleware
- **Example**: Logging → Auth → Quota → Proxy

### 2. Repository Pattern
- **Database Abstraction**: Clean interface for data access
- **Testability**: Easy to mock for testing
- **Flexibility**: Can swap database implementations
- **Example**: `database.Database` interface

### 3. Context Pattern
- **Request Context**: Carries request-scoped data
- **Cancellation**: Proper cleanup on timeout/disconnect
- **Tracing**: Request ID for end-to-end tracking
- **Example**: `request_context.RequestContext`

### 4. Worker Pool Pattern
- **Background Processing**: Async operations don't block requests
- **Resource Management**: Controlled concurrency
- **Graceful Shutdown**: Proper cleanup of workers
- **Example**: Metrics worker for cost tracking

### 5. Factory Pattern
- **Object Creation**: Centralized creation logic
- **Configuration**: Easy to configure instances
- **Example**: Logger creation, database initialization

### 6. Strategy Pattern
- **Model Resolution**: Different pricing for different models
- **Extensibility**: Easy to add new models
- **Example**: `ModelResolver` for cost calculation

## Component Relationships

### Dependency Graph
```
main.go
  ├── config.Config
  ├── database.Database
  │   └── Used by: auth, quota, metrics
  ├── amslog.Logger
  │   └── Used by: all packages
  ├── auth.Middleware
  │   ├── Depends on: database, logger
  │   └── Provides: JWT validation, rate limiting
  ├── quota.Middleware
  │   ├── Depends on: database, logger
  │   └── Provides: quota enforcement
  ├── bedrock.ProxyHandler
  │   ├── Depends on: logger, metrics
  │   └── Provides: Bedrock API proxy
  ├── metrics.Worker
  │   ├── Depends on: database, logger
  │   └── Provides: async cost tracking
  └── scheduler.Scheduler
      ├── Depends on: database, logger
      └── Provides: background tasks
```

### Data Flow
```
HTTP Request
  → Logging Middleware (adds request ID, sanitizes)
  → Auth Middleware (validates JWT, adds user context)
  → Quota Middleware (checks limits, updates usage)
  → Bedrock Handler (proxies to AWS)
  → Streaming Response (SSE with XML buffering)
  → Metrics Capture (extracts token counts)
  → Metrics Worker (calculates cost asynchronously)
  → Database (stores usage data)
```

## Critical Implementation Paths

### 1. Successful Request Path
```
1. Client sends POST /v1/messages with JWT
2. Logging middleware adds request ID and trace ID
3. Auth middleware validates JWT and extracts user info
4. Rate limiter checks if key is within limits
5. Quota middleware checks user quotas
6. Bedrock handler signs request with AWS SigV4
7. Request translated to Bedrock Converse API format
8. Response streams back to client (SSE)
9. XML buffer ensures complete tags
10. Token counts extracted from metadata event
11. Cost calculated based on model and tokens
12. Metrics queued for background worker
13. Worker updates database asynchronously
14. Request completes successfully
```

### 2. Authentication Failure Path
```
1. Client sends request with invalid/missing JWT
2. Logging middleware adds request context
3. Auth middleware rejects request
4. Rate limiter records failed attempt
5. Returns 401 Unauthorized with error details
6. Request logged with error context
7. No further processing
```

### 3. Quota Exceeded Path
```
1. Request passes authentication
2. Quota middleware checks limits
3. User has exceeded daily/monthly quota
4. Returns 401 Unauthorized (for Cline compatibility)
5. Includes rate limit headers
6. Request logged with quota error
7. No Bedrock API call made
```

### 4. Streaming Error Path
```
1. Request reaches Bedrock handler
2. AWS API call initiated
3. Streaming starts successfully
4. Error occurs mid-stream
5. Error logged with context
6. Connection closed gracefully
7. Partial usage tracked
8. Client receives error event
```

### 5. Graceful Shutdown Path
```
1. Shutdown signal received (SIGTERM/SIGINT)
2. HTTP server stops accepting new requests
3. Existing requests allowed to complete (30s timeout)
4. Background workers finish current tasks
5. Scheduler stops scheduling new tasks
6. Database connections closed
7. All goroutines cleaned up
8. Process exits cleanly
```

## Request Flow Patterns

### Authentication Flow
```
Request → Extract JWT → Validate Token → Extract User Info → 
Check Rate Limit → Attach User Context → Continue
```

### Quota Check Flow
```
Request → Get User Quotas → Check Limits → 
Update Usage Counter → Continue or Reject
```

### Proxy Flow
```
Request → Parse Body → Convert Format → Sign Request → 
Call Bedrock → Stream Response → Buffer XML → 
Track Tokens → Calculate Cost → Log Metrics → Return to Client
```

### Cost Tracking Flow
```
Response Complete → Extract Token Counts → 
Calculate Cost → Queue Metrics → 
Background Worker → Update Database