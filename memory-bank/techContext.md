# Technical Context

## Technology Stack

### Core Technologies
- **Language**: Go 1.24+ (type-safe, concurrent, performant)
- **Database**: PostgreSQL 13+ (production-ready, high concurrency)
- **AWS SDK**: AWS SDK for Go v2 (Bedrock Runtime client)
- **HTTP Server**: Go standard library `net/http`
- **Containerization**: Docker (multi-stage builds)

### Key Libraries
- `github.com/golang-jwt/jwt/v5` - JWT token handling
- `github.com/aws/aws-sdk-go-v2` - AWS Bedrock integration
- `github.com/jackc/pgx/v5` - PostgreSQL driver (high performance)
- `golang.org/x/time/rate` - Rate limiting
- `github.com/google/uuid` - UUID generation
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

## Development Setup

### Prerequisites
- Go 1.24 or higher
- PostgreSQL 13+ (for authentication and metrics)
- AWS credentials configured (for Bedrock access)
- Docker (optional, for containerized deployment)

### Environment Variables

**Server Configuration**
```bash
PORT=8080                    # HTTP server port
LOG_LEVEL=info              # Logging level (debug, info, warn, error)
LOG_FORMAT=json             # Log format (json, text)
LOG_OUTPUT=stdout           # Log output (file, stdout, both)
```

**AWS Configuration**
```bash
AWS_BEDROCK_REGION=us-east-1        # AWS region for Bedrock
AWS_BEDROCK_ACCESS_KEY=xxx          # AWS access key
AWS_BEDROCK_SECRET_KEY=xxx          # AWS secret key
AWS_BEDROCK_DEBUG=false             # Enable AWS SDK debug logging
```

**Authentication**
```bash
JWT_SECRET_ARN=arn:aws:...          # JWT secret from Secrets Manager (recommended)
# OR
JWT_SECRET_KEY=your-secret-key      # JWT signing secret (≥32 chars, OWASP requirement)
JWT_ISSUER=identity-manager         # Token issuer
JWT_AUDIENCE=bedrock-proxy          # Token audience
JWT_EXPIRATION=24h                  # Token expiration duration
```

**Database**
```bash
DB_SECRET_ARN=arn:aws:...           # DB credentials from Secrets Manager (recommended)
# OR
DB_HOST=localhost                   # PostgreSQL host
DB_PORT=5432                        # PostgreSQL port
DB_NAME=proxy                       # Database name
DB_USER=proxy_user                  # Database user
DB_PASSWORD=xxx                     # Database password
DB_SSLMODE=require                  # SSL mode (require, verify-full, disable)
DB_MAX_CONNS=25                     # Maximum connections
DB_MIN_CONNS=5                      # Minimum connections
```

**Bedrock Features**
```bash
AWS_BEDROCK_MAX_TOKENS=8192                    # Max tokens per response
AWS_BEDROCK_FORCE_PROMPT_CACHING=true          # Auto-add cache points
AWS_BEDROCK_ENABLE_COMPUTER_USE=false          # Enable Computer Use beta
AWS_BEDROCK_ENABLE_OUTPUT_REASON=false         # Enable Extended Thinking
AWS_BEDROCK_REASON_BUDGET_TOKENS=1024          # Thinking budget tokens
XML_BUFFER_MAX_SIZE=100                        # XML buffer size (chars)
```

**Quotas (Optional)**
```bash
DEFAULT_DAILY_QUOTA=1000            # Default daily request quota
DEFAULT_MONTHLY_QUOTA=30000         # Default monthly request quota
```

### Local Development

**Install dependencies:**
```bash
go mod download
```

**Run the proxy:**
```bash
go run cmd/main.go
```

**Build binary:**
```bash
go build -o bin/proxy cmd/main.go
```

**Run with custom config:**
```bash
PORT=9000 LOG_LEVEL=debug go run cmd/main.go
```

### Docker Deployment

**Build image:**
```bash
docker build -t bedrock-proxy .
```

**Run container:**
```bash
docker run -p 8080:8080 \
  -e AWS_BEDROCK_REGION=us-east-1 \
  -e AWS_BEDROCK_ACCESS_KEY=xxx \
  -e AWS_BEDROCK_SECRET_KEY=xxx \
  -e JWT_SECRET_KEY=your-secret \
  -e DB_HOST=postgres \
  -v $(pwd)/data:/app/data \
  bedrock-proxy
```

**Docker Compose:**
```yaml
version: '3.8'
services:
  proxy:
    build: .
    ports:
      - "8080:8080"
    environment:
      - AWS_BEDROCK_REGION=us-east-1
      - JWT_SECRET_ARN=arn:aws:...
      - DB_SECRET_ARN=arn:aws:...
    depends_on:
      - postgres
  
  postgres:
    image: postgres:15
    environment:
      - POSTGRES_DB=proxy
      - POSTGRES_USER=proxy_user
      - POSTGRES_PASSWORD=xxx
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

## Technical Constraints & Considerations

### Performance
- **Concurrent Requests**: Go's goroutines handle high concurrency efficiently
- **Database**: PostgreSQL performs well up to millions of requests/day
- **Memory**: Streaming responses minimize memory usage
- **CPU**: Cost calculation is lightweight; metrics worker prevents blocking
- **Latency**: XML buffer adds <1ms per chunk

### Scalability
- **Horizontal Scaling**: Stateless design allows multiple instances
- **Rate Limiting**: Currently in-memory (resets per instance); consider Redis for distributed
- **Database**: PostgreSQL supports multi-instance deployments
- **Caching**: No caching layer yet; consider Redis for response caching
- **Load Balancing**: Use ALB/NLB for multi-instance deployments

### Security
- **JWT Secrets**: Must be ≥32 characters (OWASP requirement)
- **API Keys**: Stored hashed in database
- **Request Sanitization**: Prevents log injection attacks
- **CORS**: Configurable for web clients
- **TLS**: Recommended for production (use reverse proxy like nginx/ALB)
- **AWS Secrets Manager**: Recommended for credential storage

### Reliability
- **Error Handling**: Comprehensive error handling throughout
- **Graceful Shutdown**: Proper cleanup of resources (30s timeout)
- **Connection Pooling**: Prevents database connection exhaustion
- **Context Cancellation**: Prevents goroutine leaks
- **Retry Logic**: Basic retry on transient errors (can be enhanced)
- **Health Checks**: `/health` endpoint for monitoring

### Monitoring & Observability
- **Structured Logging**: JSON format compatible with ECS
- **Request Tracing**: Unique request ID and trace ID
- **Metrics**: Token counts, costs, durations
- **Error Tracking**: Stack traces and error codes
- **Performance**: Phase-by-phase timing (sign, parse, stream, post-process)

### Supported Models & Pricing

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
- Cache Read: $0.30/1M tokens (90% discount)

### Prompt Caching
- **Cache Write**: First-time cache creation (25% premium)
- **Cache Read**: Subsequent cache hits (90% discount)
- **Configuration**: `AWS_BEDROCK_FORCE_PROMPT_CACHING` controls behavior
- **Automatic**: Adds cache points to last system block and last user message

## Development Tools

### Recommended IDE Setup
- **VS Code** with Go extension
- **GoLand** by JetBrains
- **Vim/Neovim** with vim-go

### Useful Commands
```bash
# Format code
go fmt ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Build for production
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o proxy cmd/main.go

# Run linter
golangci-lint run

# Generate mocks (if using mockgen)
mockgen -source=pkg/database/database.go -destination=pkg/database/mock_database.go
```

### Debugging
- Use `AWS_BEDROCK_DEBUG=true` for AWS SDK debug logs
- Use `LOG_LEVEL=debug` for detailed application logs
- Check `/health` endpoint for service status
- Monitor PostgreSQL connections with `SELECT * FROM pg_stat_activity;`

## Deployment Considerations

### AWS ECS/Fargate
- Use task definition with environment variables
- Store secrets in AWS Secrets Manager
- Use ALB for load balancing
- Enable CloudWatch logs
- Configure auto-scaling based on CPU/memory

### Kubernetes
- Use ConfigMaps for non-sensitive config
- Use Secrets for sensitive data
- Configure liveness and readiness probes
- Set resource limits (CPU, memory)
- Use HPA for auto-scaling

### Production Checklist
- [ ] Configure AWS Secrets Manager for credentials
- [ ] Enable TLS/HTTPS (via ALB or nginx)
- [ ] Set up CloudWatch/Prometheus monitoring
- [ ] Configure log aggregation (CloudWatch, ELK, etc.)
- [ ] Set appropriate resource limits
- [ ] Configure auto-scaling policies
- [ ] Set up alerting for errors and quota exceeded
- [ ] Test graceful shutdown behavior
- [ ] Verify database connection pooling
- [ ] Test rate limiting under load