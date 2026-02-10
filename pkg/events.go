package pkg

// Eventos de Request/Response
const (
	EventProxyRequestStart = "PROXY_REQUEST_START"
	EventProxyRequestEnd   = "PROXY_REQUEST_END"
	EventProxyRequestError = "PROXY_REQUEST_ERROR"
)

// Eventos de Bedrock
const (
	EventBedrockInvoke         = "BEDROCK_INVOKE"
	EventBedrockStreamStart    = "BEDROCK_STREAM_START"
	EventBedrockStreamComplete = "BEDROCK_STREAM_COMPLETE"
	EventBedrockError          = "BEDROCK_ERROR"
)

// Eventos de Autenticación
const (
	EventAuthJWTValidate = "AUTH_JWT_VALIDATE"
	EventAuthJWTError    = "AUTH_JWT_ERROR"
	EventAuthLogin       = "AUTH_LOGIN"
)

// Eventos de Quota
const (
	EventQuotaCheck    = "QUOTA_CHECK"
	EventQuotaExceeded = "QUOTA_EXCEEDED"
	EventQuotaUpdate   = "QUOTA_UPDATE"
)

// Eventos de Métricas
const (
	EventMetricsRecord = "METRICS_RECORD"
	EventCostCalculate = "COST_CALCULATE"
)

// Eventos de Base de Datos
const (
	EventDBQuery  = "DB_QUERY"
	EventDBUpdate = "DB_UPDATE"
	EventDBError  = "DB_ERROR"
)

// Eventos de Cache
const (
	EventCacheRead  = "CACHE_READ"
	EventCacheWrite = "CACHE_WRITE"
)

// Eventos de Sistema
const (
	EventLoggerInit     = "LOGGER_INIT"
	EventServerStart    = "SERVER_START"
	EventServerShutdown = "SERVER_SHUTDOWN"
)