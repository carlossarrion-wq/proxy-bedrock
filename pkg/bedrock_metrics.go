package pkg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bedrock-proxy-test/pkg/auth"
	"bedrock-proxy-test/pkg/database"
	"bedrock-proxy-test/pkg/metrics"
)

// processMetrics procesa las métricas en una goroutine separada
func (this *BedrockClient) processMetrics(ctx context.Context, user *auth.UserContext, mc *MetricsCapture, startTime time.Time) {
	// Finalizar captura de métricas
	mc.Finalize()
	
	// Calcular tiempo total de procesamiento
	processingTimeMS := int(time.Since(startTime).Milliseconds())
	
	// Obtener métricas capturadas
	metric := mc.GetMetrics()
	
	// Completar información del usuario y tiempo
	metric.UserID = user.UserID
	metric.Team = user.Team
	metric.Person = user.Person
	metric.RequestTimestamp = startTime
	metric.AWSRegion = this.config.Region
	metric.ProcessingTimeMS = processingTimeMS
	
	// Calcular coste
	cost, err := metrics.CalculateCost(
		metric.ModelID,
		int64(metric.TokensInput),
		int64(metric.TokensOutput),
	)
	if err != nil {
		Log.Errorf("Failed to calculate cost: %v", err)
		cost = 0.0
	}
	metric.CostUSD = cost
	
	// 1. Guardar métrica (asíncrono via worker)
	if err := this.metricsWorker.RecordMetric(metric); err != nil {
		Log.Errorf("Failed to record metric: %v", err)
	}
	
	// 2. Actualizar quotas (síncrono - importante para límites)
	if err := this.db.UpdateQuotaAndCounters(ctx, user.UserID, cost); err != nil {
		Log.Errorf("Failed to update quota: %v", err)
	}
	
	// 3. Verificar si debe bloquearse el usuario
	if err := this.db.CheckAndBlockUser(ctx, user.UserID); err != nil {
		Log.Errorf("Failed to check/block user: %v", err)
	}
	
	Log.Infof("[METRICS] User: %s | Tokens: %d/%d | Cost: $%.6f | Time: %dms",
		user.UserID, metric.TokensInput, metric.TokensOutput, cost, processingTimeMS)
}

// MetricsCapture captura información de métricas mientras hace streaming
type MetricsCapture struct {
	http.ResponseWriter
	buffer           bytes.Buffer
	statusCode       int
	inputTokens      int
	outputTokens     int
	cacheReadTokens  int
	cacheWriteTokens int
	modelID          string
	requestID        string
	sourceIP         string
	userAgent        string
	hasError         bool
	errorMessage     string
}

func NewMetricsCapture(w http.ResponseWriter, modelID, requestID string, r *http.Request) *MetricsCapture {
	sourceIP := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		sourceIP = forwarded
	}
	
	return &MetricsCapture{
		ResponseWriter: w,
		statusCode:     200,
		modelID:        modelID,
		requestID:      requestID,
		sourceIP:       sourceIP,
		userAgent:      r.Header.Get("User-Agent"),
	}
}

func (mc *MetricsCapture) Write(data []byte) (int, error) {
	// Acumular en buffer para parsing posterior (sin bloquear)
	mc.buffer.Write(data)
	
	// Enviar al cliente inmediatamente SIN flush adicional
	// El flush lo maneja handleBedrockStream
	return mc.ResponseWriter.Write(data)
}

func (mc *MetricsCapture) WriteHeader(statusCode int) {
	mc.statusCode = statusCode
	mc.ResponseWriter.WriteHeader(statusCode)
}

func (mc *MetricsCapture) Flush() {
	if flusher, ok := mc.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (mc *MetricsCapture) Finalize() {
	mc.parseSSEEvent(mc.buffer.String())
	Log.Debugf("Metrics finalized: input=%d, output=%d, cache_read=%d, cache_write=%d",
		mc.inputTokens, mc.outputTokens, mc.cacheReadTokens, mc.cacheWriteTokens)
}

func (mc *MetricsCapture) parseSSEEvent(data string) {
	lines := strings.Split(data, "\n")
	
	for i, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "data: ") {
				jsonData := strings.TrimPrefix(lines[i+1], "data: ")
				mc.extractTokensFromEvent(eventType, jsonData)
			}
		}
	}
}

func (mc *MetricsCapture) extractTokensFromEvent(eventType, jsonData string) {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		return
	}
	
	switch eventType {
	case "message_start":
		if message, ok := event["message"].(map[string]interface{}); ok {
			if usage, ok := message["usage"].(map[string]interface{}); ok {
				if inputTokens, ok := usage["input_tokens"].(float64); ok {
					mc.inputTokens = int(inputTokens)
				}
				// Capturar cache tokens de message_start
				if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
					mc.cacheReadTokens = int(cacheRead)
				}
				if cacheCreation, ok := usage["cache_creation_input_tokens"].(float64); ok {
					mc.cacheWriteTokens = int(cacheCreation)
				}
			}
		}
		
	case "message_delta":
		// Capturar tokens finales de output desde message_delta (formato Anthropic)
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				mc.outputTokens = int(outputTokens)
				Log.Infof("[METRICS_CAPTURE] Output tokens from message_delta: %d", mc.outputTokens)
			}
		}

	case "ping":
		// Evento ping contiene todos los tokens finales (para captura de métricas)
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			if inputTokens, ok := usage["input_tokens"].(float64); ok {
				mc.inputTokens = int(inputTokens)
			}
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				mc.outputTokens = int(outputTokens)
			}
			if cacheCreation, ok := usage["cache_creation_input_tokens"].(float64); ok {
				mc.cacheWriteTokens = int(cacheCreation)
			}
			if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
				mc.cacheReadTokens = int(cacheRead)
			}
			Log.Infof("[METRICS_CAPTURE] Tokens from ping: input=%d, output=%d, cache_read=%d, cache_write=%d",
				mc.inputTokens, mc.outputTokens, mc.cacheReadTokens, mc.cacheWriteTokens)
		}

	case "message_stop":
		// message_stop ya no contiene usage en formato Anthropic
		// Los tokens finales ya fueron capturados en ping o message_delta
		Log.Infof("[METRICS_CAPTURE] Final metrics: input=%d, output=%d, cache_read=%d, cache_write=%d",
			mc.inputTokens, mc.outputTokens, mc.cacheReadTokens, mc.cacheWriteTokens)

	case "error":
		mc.hasError = true
		if errorData, ok := event["error"].(map[string]interface{}); ok {
			if errType, ok := errorData["type"].(string); ok {
				if errMsg, ok := errorData["message"].(string); ok {
					mc.errorMessage = fmt.Sprintf("%s: %s", errType, errMsg)
				}
			}
		}
	}
}

func (mc *MetricsCapture) GetMetrics() *database.MetricData {
	return &database.MetricData{
		ModelID:             mc.modelID,
		RequestID:           mc.requestID,
		SourceIP:            mc.sourceIP,
		UserAgent:           mc.userAgent,
		TokensInput:         mc.inputTokens,
		TokensOutput:        mc.outputTokens,
		TokensCacheRead:     mc.cacheReadTokens,
		TokensCacheCreation: mc.cacheWriteTokens,
		ResponseStatus:      mc.getStatusString(),
		ErrorMessage:        mc.errorMessage,
	}
}

func (mc *MetricsCapture) getStatusString() string {
	if mc.hasError {
		return "error"
	}
	if mc.statusCode >= 200 && mc.statusCode < 300 {
		return "success"
	}
	return fmt.Sprintf("http_%d", mc.statusCode)
}
