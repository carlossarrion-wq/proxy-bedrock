# Product Context

## Why This Project Exists

### The Problem
Organizations using AWS Bedrock for Claude models face a challenge: most AI development tools (like Cline) are built for Anthropic's direct API, not AWS Bedrock's API. The two APIs have different:
- Request/response formats
- Authentication mechanisms
- Streaming protocols
- Tool use implementations

### The Solution
This proxy acts as a translation layer, allowing developers to:
- Use familiar tools (Cline) with AWS Bedrock
- Centralize authentication and cost control
- Track usage and enforce quotas
- Maintain compatibility with Anthropic's API format

## How It Should Work

### User Experience
1. **Developer configures Cline** with proxy endpoint and JWT token
2. **Cline sends requests** in Anthropic format to proxy
3. **Proxy translates** request to Bedrock format
4. **Bedrock processes** and streams response
5. **Proxy translates back** to Anthropic format
6. **Cline receives** response as if from Anthropic directly

### Key Features

**Transparent Translation**
- Developers don't need to change their code
- Full compatibility with Anthropic's Messages API
- Support for all Claude model features (tools, images, caching)

**Authentication & Authorization**
- JWT tokens with user/team information
- Rate limiting per API key
- Daily and monthly quota enforcement
- Automatic quota reset at midnight UTC

**Cost Management**
- Real-time cost calculation per request
- Token usage tracking (input, output, cache)
- Per-user and per-team cost visibility
- Quota enforcement to prevent overruns

**Observability**
- Structured JSON logging
- Request tracing with unique IDs
- Performance metrics per phase
- Error tracking with stack traces

## Problems It Solves

1. **API Incompatibility**: Bridges the gap between Anthropic and Bedrock APIs
2. **Cost Control**: Prevents unexpected AWS bills through quotas
3. **Authentication**: Centralizes access control with JWT
4. **Monitoring**: Provides visibility into usage patterns
5. **XML Streaming Issues**: Fixes Bedrock's XML tag splitting problem

## User Experience Goals

### For Developers
- **Zero friction**: Works with existing Cline setup
- **Fast**: Minimal latency overhead (<100ms)
- **Reliable**: Handles errors gracefully
- **Transparent**: Clear error messages and rate limit headers

### For Administrators
- **Controllable**: Easy quota management
- **Visible**: Comprehensive usage metrics
- **Secure**: Strong authentication and rate limiting
- **Maintainable**: Clear logs and monitoring

### For Organizations
- **Cost-effective**: Prevents budget overruns
- **Compliant**: Audit trail of all requests
- **Scalable**: Handles multiple teams and users
- **Flexible**: Configurable limits per user/team