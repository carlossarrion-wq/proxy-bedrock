# Active Context

## Current Focus
**Project Status**: Production-ready with all core features implemented and operational

**Current State**: The proxy is fully functional and successfully:
- Translates between Anthropic and Bedrock API formats
- Streams responses in real-time with SSE
- Authenticates requests with JWT validation
- Enforces quotas and rate limits
- Tracks costs and usage metrics
- Handles tool use for Claude models

## Recent Changes

### Completed Features ✅
- **Authentication System**: JWT validation with rate limiting (5 attempts/min per IP, 10/min per token)
- **Quota Management**: Daily/monthly limits with automatic reset at midnight UTC
- **Streaming Support**: Full SSE streaming with intelligent XML buffering
- **Tool Use**: Complete support for Claude's tool use capabilities
- **Prompt Caching**: Automatic cache point insertion for AWS Bedrock
- **Cost Tracking**: Real-time calculation with model-specific pricing
- **Structured Logging**: amslog package with JSON output and sanitization
- **Metrics Worker**: Asynchronous processing to avoid blocking requests
- **Scheduler**: Background tasks for quota resets
- **Docker**: Multi-stage build with health checks
- **Error Propagation Fix (2026-03-25)**: Bedrock errors now sent as SSE events in Anthropic format for proper Cline display

### Key Implementation Details
- **XML Buffer**: 100-character buffer prevents tag splitting (configurable via `XML_BUFFER_MAX_SIZE`)
- **Buffering Strategy**: Selective buffering - only sends `message_start` after receiving real token counts from metadata
- **Database**: PostgreSQL with connection pooling (5-25 connections)
- **Graceful Shutdown**: Proper cleanup of workers and connections

## Next Steps

### Immediate Priorities
1. **Testing**: Add comprehensive unit and integration tests
2. **Documentation**: Create API documentation and deployment guides
3. **Monitoring**: Consider Prometheus metrics export

### Future Enhancements
1. **Multi-region Support**: Route to different AWS regions
2. **Caching Layer**: Redis integration for response caching
3. **Retry Logic**: Exponential backoff for transient errors
4. **WebSocket Support**: Alternative to SSE for real-time streaming
5. **Admin Dashboard**: Web UI for monitoring and management

## Active Decisions & Patterns

### Architecture Decisions
- **PostgreSQL over SQLite**: Chosen for production scalability and concurrent access
- **JWT Authentication**: Stateless, secure, with configurable expiration
- **Middleware Chain**: Clean separation (auth → quota → logging → proxy)
- **Async Workers**: Metrics processing doesn't block request handling
- **AWS Secrets Manager**: Recommended for credential management

### Key Implementation Patterns
1. **Request Context**: Custom context with request ID, user info, and timing
2. **Structured Logging**: Consistent JSON format with ECS compatibility
3. **Graceful Shutdown**: Proper cleanup with 30-second timeout
4. **Error Handling**: Consistent error responses with proper HTTP status codes
5. **Cost Tracking**: Real-time calculation based on token usage

### Configuration Philosophy
- Environment variables for all configurable values
- Sensible defaults for development
- AWS Secrets Manager for production credentials
- No hardcoded secrets or sensitive data

## Important Learnings & Insights

### Security Considerations
- ✅ JWT secrets must be ≥32 characters (OWASP requirement)
- ✅ Rate limiting prevents brute force attacks
- ✅ Request sanitization prevents log injection
- ✅ Proper CORS configuration for web clients
- ⚠️ Consider request signing for additional security layer

### Performance Insights
- **XML Buffering**: Critical for proper Claude streaming - prevents tool detection failures
- **Connection Pooling**: Significantly improves concurrent request handling
- **Background Workers**: Prevent blocking on metrics collection
- **Context Cancellation**: Prevents goroutine leaks on client disconnect

### Operational Patterns
- **Structured Logging**: Makes debugging 10x easier
- **Request IDs**: Enable end-to-end request tracing
- **Cost Tracking**: Provides visibility into usage patterns
- **Quota Management**: Prevents unexpected AWS bills

### Code Quality
- **Package Separation**: Clear boundaries improve maintainability
- **Middleware Pattern**: Easy to add/remove features
- **Comprehensive Error Handling**: Improves reliability
- **Type Safety**: Go's type system catches many bugs at compile time

## Critical Implementation Notes

### XML Buffer Behavior
The XML buffer is essential for Cline compatibility:
- Bedrock can split XML tags across chunks (e.g., `<write_fi` + `le>`)
- Buffer holds up to 100 characters to detect incomplete tags
- Only releases content when tags are complete
- Adds <1ms latency per chunk
- Configurable via `XML_BUFFER_MAX_SIZE` environment variable

### Prompt Caching Strategy
Two modes controlled by `AWS_BEDROCK_FORCE_PROMPT_CACHING`:
- **true** (default): Automatically adds cache points to last system block and last user message
- **false**: Respects client's cache_control directives

### Token Counting
- Input tokens: Includes system prompt + messages
- Output tokens: Generated response
- Cache write tokens: First-time cache creation (25% premium)
- Cache read tokens: Subsequent cache hits (90% discount)

### Inference Profiles
- Each user must have `default_inference_profile` in JWT claims
- This is the ARN of the Bedrock inference profile to use
- Proxy uses this ARN directly (no model mapping needed)

### Error Propagation to Cline (Fix 2026-03-25)
**Problem Solved**: Bedrock errors were logged but not sent to Cline, causing generic error messages

**Implementation**:
- Added `sendSSEError()` function to send errors in Anthropic SSE format
- Errors sent when `ConverseStream()` fails to start
- Errors sent when stream encounters errors during processing
- Format: `event: error\ndata: {"type":"error","error":{"type":"api_error","message":"..."}}`

**Result**: Cline now displays actual Bedrock errors like:
- "prompt is too long: 210727 tokens > 200000 maximum"
- "ValidationException" with full details
- Request IDs for debugging

**Before**: Generic message "This may indicate a failure in Cline's thought process..."
**After**: Actual error message from Bedrock with actionable information
