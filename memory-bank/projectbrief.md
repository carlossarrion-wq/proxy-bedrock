# Project Brief: Bedrock Proxy

## Project Overview
A high-performance HTTP proxy that translates Anthropic's Messages API format to AWS Bedrock's Converse API format, enabling tools like Cline to use Claude models hosted on AWS Bedrock.

## Core Objectives
1. **API Translation**: Seamless conversion between Anthropic and Bedrock API formats
2. **Authentication & Security**: JWT-based authentication with rate limiting and quota management
3. **Streaming Support**: Real-time SSE streaming with intelligent XML buffering
4. **Cost Control**: Comprehensive usage tracking and quota enforcement
5. **Observability**: Structured logging and metrics collection

## Key Requirements

### Functional Requirements
- Translate Anthropic Messages API requests to Bedrock Converse API
- Support streaming responses with Server-Sent Events (SSE)
- Handle tool use (function calling) for Claude models
- Support image inputs and prompt caching
- Authenticate requests using JWT tokens
- Enforce daily and monthly usage quotas
- Track token usage and calculate costs per model

### Non-Functional Requirements
- High performance with minimal latency overhead
- Graceful handling of streaming interruptions
- Proper XML tag buffering to prevent parsing issues
- Comprehensive error handling and logging
- Production-ready with Docker containerization

## Success Criteria
- Successfully proxy requests from Cline to AWS Bedrock
- Maintain streaming performance with <100ms overhead
- Accurate cost tracking within 1% margin
- Zero data loss during streaming
- 99.9% uptime for production deployments

## Target Users
- Development teams using Cline with AWS Bedrock
- Organizations requiring cost control for LLM usage
- Teams needing centralized authentication for AI services

## Technical Constraints
- Must use AWS Bedrock Converse API (not InvokeModel)
- Must support Claude 3 and 3.5 model families
- Must handle Anthropic's cache_control format
- Must work with existing Cline configurations
- PostgreSQL for production persistence