package pkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	bedrockRuntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/google/uuid"

	"bedrock-proxy-test/pkg/amslog"
	"bedrock-proxy-test/pkg/auth"
	"bedrock-proxy-test/pkg/database"
	"bedrock-proxy-test/pkg/metrics"
	"bedrock-proxy-test/pkg/quota"
)

// Constantes para configuración de Bedrock
const (
	DefaultMaxTokens      = 8192
	DefaultTemperature    = 0.0
	MaxMessagesPerRequest = 1000
)

type BedrockConfig struct {
	AccessKey                string            `json:"access_key"`
	SecretKey                string            `json:"secret_key"`
	Region                   string            `json:"region"`
	AnthropicVersionMappings map[string]string `json:"anthropic_version_mappings"`
	ModelMappings            map[string]string `json:"model_mappings"`
	AnthropicDefaultModel    string            `json:"anthropic_default_model"`
	AnthropicDefaultVersion  string            `json:"anthropic_default_version"`
	EnableComputerUse        bool              `json:"enable_computer_use"`
	EnableOutputReason       bool              `json:"enable_output_reasoning"`
	ReasonBudgetTokens       int               `json:"reason_budget_tokens"`
	MaxTokens                int               `json:"max_tokens"`
	DEBUG                    bool              `json:"debug,omitempty"`
}

type ThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

func ParseMappingsFromStr(raw string) map[string]string {
	mappings := map[string]string{}
	pairs := strings.Split(raw, ",")
	// Iterate over each key-value pair
	for _, pair := range pairs {
		// Split key and value by equals sign
		kv := strings.Split(pair, "=")
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			mappings[key] = value
		}
	}

	return mappings
}

func LoadBedrockConfigWithEnv() *BedrockConfig {
	config := &BedrockConfig{
		AccessKey:                os.Getenv("AWS_BEDROCK_ACCESS_KEY"),
		SecretKey:                os.Getenv("AWS_BEDROCK_SECRET_KEY"),
		Region:                   os.Getenv("AWS_BEDROCK_REGION"),
		ModelMappings:            ParseMappingsFromStr(os.Getenv("AWS_BEDROCK_MODEL_MAPPINGS")),
		AnthropicVersionMappings: ParseMappingsFromStr(os.Getenv("AWS_BEDROCK_ANTHROPIC_VERSION_MAPPINGS")),
		AnthropicDefaultModel:    os.Getenv("AWS_BEDROCK_ANTHROPIC_DEFAULT_MODEL"),
		AnthropicDefaultVersion:  os.Getenv("AWS_BEDROCK_ANTHROPIC_DEFAULT_VERSION"),
		EnableComputerUse:        os.Getenv("AWS_BEDROCK_ENABLE_COMPUTER_USE") == "true",
		EnableOutputReason:       os.Getenv("AWS_BEDROCK_ENABLE_OUTPUT_REASON") == "true",
		ReasonBudgetTokens:       1024,
		MaxTokens:                0,
		DEBUG:                    os.Getenv("AWS_BEDROCK_DEBUG") == "true",
	}

	budget := os.Getenv("AWS_BEDROCK_REASON_BUDGET_TOKENS")
	if len(budget) > 0 {
		if tokens, err := strconv.Atoi(budget); err == nil {
			config.ReasonBudgetTokens = tokens
		}
	}

	maxTokens := os.Getenv("AWS_BEDROCK_MAX_TOKENS")
	if len(maxTokens) > 0 {
		if tokens, err := strconv.Atoi(maxTokens); err == nil {
			config.MaxTokens = tokens
		}
	}

	return config
}

type BedrockClient struct {
	config        *BedrockConfig
	client        *bedrockRuntime.Client
	db            *database.Database
	metricsWorker *metrics.MetricsWorker
	quotaMw       *quota.QuotaMiddleware
}

type ModelInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	ID      string `json:"id"`
}

// BedrockFoundationModel represents a model from Bedrock API
type BedrockFoundationModel struct {
	ModelId                    string   `json:"modelId"`
	ModelName                  string   `json:"modelName"`
	ProviderName               string   `json:"providerName"`
	InputModalities            []string `json:"inputModalities"`
	OutputModalities           []string `json:"outputModalities"`
	ResponseStreamingSupported bool     `json:"responseStreamingSupported"`
}

// BedrockModelsResponse represents the response from Bedrock foundation models API
type BedrockModelsResponse struct {
	ModelSummaries []BedrockFoundationModel `json:"modelSummaries"`
}

// ModelValidationResult represents the validation result for a model mapping
type ModelValidationResult struct {
	ConfigModel    string `json:"config_model"`
	BedrockModelId string `json:"bedrock_model_id"`
	ModelName      string `json:"model_name,omitempty"` // Optional, can be used to store the model name if available
	IsValid        bool   `json:"is_valid"`
	Available      bool   `json:"available"`
}

func (this *BedrockClient) ListModels() []ModelInfo {
	models := make([]ModelInfo, 0, len(this.config.AnthropicVersionMappings))
	for name, version := range this.config.AnthropicVersionMappings {
		models = append(models, ModelInfo{ID: name, Version: version, Name: fmt.Sprintf("%s-%s", name, version)})
	}
	return models
}

// GetBedrockAvailableModels fetches available models from Bedrock API
func (this *BedrockClient) GetBedrockAvailableModels() ([]BedrockFoundationModel, error) {
	// Create the API endpoint URL - use bedrock service, not bedrock-runtime
	apiEndpoint := fmt.Sprintf("https://bedrock.%s.amazonaws.com/foundation-models", this.config.Region)

	// Create HTTP request
	req, err := http.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Sign the request using AWS v4 signature
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithRegion(this.config.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			this.config.AccessKey,
			this.config.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	signer := v4.NewSigner()
	credentialList, err := cfg.Credentials.Retrieve(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %v", err)
	}

	// Sign the request
	hash := sha256.Sum256([]byte{})
	payloadHash := hex.EncodeToString(hash[:])
	err = signer.SignHTTP(context.TODO(), credentialList, req, payloadHash, "bedrock", cfg.Region, time.Now(), func(options *v4.SignerOptions) {
		if this.config.DEBUG {
			options.LogSigning = true
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %v", err)
	}

	// Execute the request
	httpClient := http.DefaultClient
	if this.config.DEBUG {
		httpClient = &http.Client{
			Transport: loggingRoundTripper{
				wrapped: http.DefaultTransport,
			},
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse the response
	var modelsResponse BedrockModelsResponse
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&modelsResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return modelsResponse.ModelSummaries, nil
}

// ValidateModelMappings validates the configured model mappings against available Bedrock models
func (this *BedrockClient) ValidateModelMappings() ([]ModelValidationResult, error) {
	// Get available models from Bedrock
	availableModels, err := this.GetBedrockAvailableModels()
	if err != nil {
		return nil, fmt.Errorf("failed to get available models: %v", err)
	}

	// Create a map for quick lookup
	availableModelIds := make(map[string]string)
	for _, model := range availableModels {
		availableModelIds[model.ModelId] = model.ModelName
	}

	// Validate each mapping
	var results []ModelValidationResult
	for configModel, bedrockModelId := range this.config.ModelMappings {
		modelName, ok := availableModelIds[bedrockModelId]
		if ok {
			results = append(results, ModelValidationResult{
				ConfigModel:    configModel,
				BedrockModelId: bedrockModelId,
				IsValid:        ok,        // Mapping exists in config
				ModelName:      modelName, // Optional, can be filled if needed
				Available:      ok,
			})
		}
	}

	return results, nil
}

// GetMergedModelList returns a combined list of configured and available models
func (this *BedrockClient) GetMergedModelList() ([]ModelInfo, error) {
	// Get validation results
	validationResults, err := this.ValidateModelMappings()
	if err != nil {
		if this.config.DEBUG {
			Logger.Error(amslog.Event{
				Name:    "MODEL_VALIDATION_ERROR",
				Message: "Failed to validate model mappings",
				Error: &amslog.ErrorInfo{
					Type:    "ValidationError",
					Message: err.Error(),
				},
			})
		}
		// Fall back to config-only models
		return this.ListModels(), nil
	}

	// Create model list from validation results
	var models []ModelInfo
	for _, result := range validationResults {
		if result.Available {
			// Use the config model name and extract version from mapping
			version := this.config.AnthropicDefaultVersion
			if mappedVersion, ok := this.config.AnthropicVersionMappings[result.ConfigModel]; ok {
				version = mappedVersion
			}

			models = append(models, ModelInfo{
				Name:    result.ModelName,
				Version: version,
				ID:      result.ConfigModel,
			})
		} else if this.config.DEBUG {
			Logger.Warning(amslog.Event{
				Name:    "MODEL_NOT_AVAILABLE",
				Message: "Model not available in Bedrock",
				Fields: map[string]interface{}{
					"config_model":    result.ConfigModel,
					"bedrock_model_id": result.BedrockModelId,
				},
			})
		}
	}

	// If no valid models found, fall back to config models
	if len(models) == 0 {
		if this.config.DEBUG {
			Logger.Warning(amslog.Event{
				Name:    "NO_VALID_MODELS",
				Message: "No valid models found from Bedrock API, falling back to config models",
			})
		}
		return this.ListModels(), nil
	}

	return models, nil
}

// Custom RoundTripper for logging requests and responses
type loggingRoundTripper struct {
	wrapped http.RoundTripper
}

func (l loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log request (DEBUG mode only)
	reqDump, _ := httputil.DumpRequestOut(req, true)
	Logger.Debug(amslog.Event{
		Name:    "HTTP_REQUEST_DUMP",
		Message: "HTTP request details",
		Fields: map[string]interface{}{
			"request": string(reqDump),
		},
	})

	// Send request
	resp, err := l.wrapped.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Log response (DEBUG mode only)
	respDump, _ := httputil.DumpResponse(resp, true)
	Logger.Debug(amslog.Event{
		Name:    "HTTP_RESPONSE_DUMP",
		Message: "HTTP response details",
		Fields: map[string]interface{}{
			"response": string(respDump),
		},
	})

	// Important: recreate response body since DumpResponse consumes it
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	return resp, nil
}

func NewBedrockClient(config *BedrockConfig) *BedrockClient {
	staticProvider := credentials.NewStaticCredentialsProvider(config.AccessKey, config.SecretKey, "")

	opt := []func(*awsConfig.LoadOptions) error{
		awsConfig.WithRegion(config.Region),
		awsConfig.WithCredentialsProvider(staticProvider),
	}

	if config.DEBUG {
		httpClient := &http.Client{
			Transport: loggingRoundTripper{
				wrapped: http.DefaultTransport,
			},
		}
		opt = append(opt, awsConfig.WithHTTPClient(httpClient))
	}

	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(), opt...)

	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}

	return &BedrockClient{
		config: config,
		client: bedrockRuntime.NewFromConfig(cfg),
	}
}

// SetDependencies establece las dependencias para post-processing
func (this *BedrockClient) SetDependencies(db *database.Database, mw *metrics.MetricsWorker, qm *quota.QuotaMiddleware) {
	this.db = db
	this.metricsWorker = mw
	this.quotaMw = qm
}

func (this *BedrockClient) GetModelMappings(source string) (string, error) {
	if len(this.config.ModelMappings) > 0 {
		if target, ok := this.config.ModelMappings[source]; ok {
			return target, nil
		}
	}

	return this.config.AnthropicDefaultModel, errors.New(fmt.Sprintf("model %s not found in model mappings", source))
}

// getKeys returns the keys of a map as a slice
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (this *BedrockClient) SignRequest(request *http.Request, inferenceProfileARN string) (*http.Request, bool, error) {
	contentType := request.Header.Get("Content-Type")
	cloneReq := request
	isStream := false
	var bodyBuff bytes.Buffer
	reader := io.TeeReader(request.Body, &bodyBuff)

	// Validar que tenemos un inference profile ARN
	if inferenceProfileARN == "" {
		return nil, false, fmt.Errorf("no inference profile ARN provided - user must have default_inference_profile in JWT")
	}

	if this.config.DEBUG {
		Logger.Debug(amslog.Event{
			Name:    "INFERENCE_PROFILE_SELECTED",
			Message: "Using inference profile from JWT",
			Fields: map[string]interface{}{
				"profile_arn": inferenceProfileARN,
			},
		})
	}

	if strings.Contains(contentType, "json") {
		decoder := json.NewDecoder(reader)
		wrapper := make(map[string]interface{})
		err := decoder.Decode(&wrapper)
		if err != nil {
			return request, false, err
		}

		// Detectar si es streaming
		if srcStream, ok := wrapper["stream"]; ok {
			if _stream, ok := srcStream.(bool); ok {
				isStream = _stream
			}
		}

		wrapper["anthropic_version"] = this.config.AnthropicDefaultVersion
		delete(wrapper, "model")
		delete(wrapper, "stream")

		if this.config.EnableComputerUse {
			wrapper["anthropic_beta"] = "computer-use-2024-10-22"
		}

		if _, ok := wrapper["thinking"]; !ok && this.config.EnableOutputReason {
			wrapper["thinking"] = &ThinkingConfig{
				Type:         "enabled",
				BudgetTokens: this.config.ReasonBudgetTokens,
			}
		}

		if !this.config.EnableOutputReason {
			delete(wrapper, "thinking")
		}

		// Apply max_tokens logic: use config value if it exists and is > 0, otherwise keep the incoming value
		if this.config.MaxTokens > 0 {
			wrapper["max_tokens"] = this.config.MaxTokens
		}

		newBody, err := json.Marshal(wrapper)
		if err != nil {
			return request, false, err
		}

		bodyBuff = *bytes.NewBuffer(newBody)

		cloneReq = &http.Request{
			Method: request.Method,
			URL:    request.URL,
			Proto:  request.Proto,
			Header: request.Header.Clone(),
			Body:   io.NopCloser(bytes.NewBuffer(bodyBuff.Bytes())),
		}
	}

	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithRegion(this.config.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			this.config.AccessKey,
			this.config.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, false, err
	}

	// Usar el ARN del inference profile directamente
	bedrockRuntimeEndPoint := fmt.Sprintf(`https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke`, this.config.Region, url.QueryEscape(inferenceProfileARN))
	if isStream {
		bedrockRuntimeEndPoint = fmt.Sprintf(`https://bedrock-runtime.%s.amazonaws.com/model/%s/invoke-with-response-stream`, this.config.Region, url.QueryEscape(inferenceProfileARN))
	}

	preSignReq, err := http.NewRequest("POST", bedrockRuntimeEndPoint, cloneReq.Body)
	if err != nil {
		return nil, false, err
	}
	preSignReq.Header.Set("Content-Type", contentType)
	preSignReq.ContentLength = int64(bodyBuff.Len())

	signer := v4.NewSigner()

	// Retrieve credentials
	credentialList, err := cfg.Credentials.Retrieve(context.TODO())
	if err != nil {
		return nil, false, err
	}

	hash := sha256.Sum256(bodyBuff.Bytes())
	payloadHash := hex.EncodeToString(hash[:])
	// Sign request
	err = signer.SignHTTP(context.TODO(), credentialList, preSignReq, payloadHash, "bedrock", cfg.Region, time.Now(), func(options *v4.SignerOptions) {
		if this.config.DEBUG {
			options.LogSigning = true
		}
	})
	if err != nil {
		return nil, false, err
	}

	return preSignReq, isStream, nil
}

// convertSystemBlocksWithCache convierte bloques de system de Anthropic a Bedrock con soporte para cache_control
func convertSystemBlocksWithCache(systemBlocks []interface{}) []types.SystemContentBlock {
	var result []types.SystemContentBlock
	
	for _, block := range systemBlocks {
		blockMap, ok := block.(map[string]interface{})
		if !ok {
			continue
		}
		
		text, _ := blockMap["text"].(string)
		
		// Añadir bloque de texto
		textBlock := &types.SystemContentBlockMemberText{
			Value: text,
		}
		result = append(result, textBlock)
		
		// Verificar si tiene cache_control - si sí, añadir cache point DESPUÉS
		if cacheControl, ok := blockMap["cache_control"].(map[string]interface{}); ok {
			if cacheType, ok := cacheControl["type"].(string); ok && cacheType == "ephemeral" {
				// Insertar cache point como bloque separado DESPUÉS del texto
				cachePointBlock := &types.SystemContentBlockMemberCachePoint{
					Value: types.CachePointBlock{
						Type: types.CachePointTypeDefault,
					},
				}
				result = append(result, cachePointBlock)
			}
		}
	}
	
	return result
}

// convertAnthropicToBedrockMessages convierte mensajes de formato Anthropic a formato Bedrock con soporte para cache_control
func convertAnthropicToBedrockMessages(anthropicMessages []interface{}) ([]types.Message, error) {
	var bedrockMessages []types.Message
	
	for _, msg := range anthropicMessages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		
		// Extraer role
		roleStr, _ := msgMap["role"].(string)
		var role types.ConversationRole
		if roleStr == "user" {
			role = types.ConversationRoleUser
		} else {
			role = types.ConversationRoleAssistant
		}
		
		// Extraer content
		var contentBlocks []types.ContentBlock
		content := msgMap["content"]
		
		if contentStr, ok := content.(string); ok {
			// Content es un string simple
			contentBlocks = []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: contentStr,
				},
			}
		} else if contentArray, ok := content.([]interface{}); ok {
			// Content es un array de bloques
			for _, block := range contentArray {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				
				blockType, _ := blockMap["type"].(string)
				switch blockType {
				case "text":
					if text, ok := blockMap["text"].(string); ok {
						// Añadir bloque de texto
						textBlock := &types.ContentBlockMemberText{
							Value: text,
						}
						contentBlocks = append(contentBlocks, textBlock)
						
						// Verificar si tiene cache_control - si sí, añadir cache point DESPUÉS
						if cacheControl, ok := blockMap["cache_control"].(map[string]interface{}); ok {
							if cacheType, ok := cacheControl["type"].(string); ok && cacheType == "ephemeral" {
								// Insertar cache point como bloque separado DESPUÉS del texto
								cachePointBlock := &types.ContentBlockMemberCachePoint{
									Value: types.CachePointBlock{
										Type: types.CachePointTypeDefault,
									},
								}
								contentBlocks = append(contentBlocks, cachePointBlock)
							}
						}
					}
				case "image":
					// Manejar imágenes si es necesario
					// Por ahora las ignoramos
				}
			}
		}
		
		if len(contentBlocks) > 0 {
			bedrockMessages = append(bedrockMessages, types.Message{
				Role:    role,
				Content: contentBlocks,
			})
		}
	}
	
	return bedrockMessages, nil
}

func (this *BedrockClient) handleBedrockStreamConverse(w http.ResponseWriter, client *bedrockRuntime.Client, modelID string, systemBlocks []types.SystemContentBlock, messages []types.Message, maxTokens int32, toolConfig *types.ToolConfiguration, toolChoice types.ToolChoice) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}
	
	flusher.Flush()

	// Crear comando ConverseStream con system blocks que incluyen cache points
	// IMPORTANTE: NO enviamos toolConfig porque Cline no lo hace cuando se conecta directamente
	// Cline usa prompt engineering (tools descritas en system prompt) + XML parsing
	input := &bedrockRuntime.ConverseStreamInput{
		ModelId:  &modelID,
		Messages: messages,
		System:   systemBlocks,
		InferenceConfig: &types.InferenceConfiguration{
			MaxTokens:   aws.Int32(maxTokens),
			Temperature: aws.Float32(DefaultTemperature),
		},
	}

	// Ejecutar streaming
	output, err := client.ConverseStream(context.Background(), input)
	if err != nil {
		return fmt.Errorf("failed to start converse stream: %w", err)
	}

	eventCount := 0
	stream := output.GetStream()
	
	// Variables para capturar métricas de uso y buffering selectivo
	var inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int32
	var messageStartReceived bool
	var messageStartSent bool
	
	// Crear buffer para evitar cortar tags XML con configuración
	bufferConfig := LoadXMLBufferConfigWithEnv()
	xmlBuffer := NewXMLTagBuffer(bufferConfig.MaxBufferSize)
	
	for {
		event, ok := <-stream.Events()
		if !ok {
			break
		}

		eventCount++

		switch e := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// NO enviar message_start aún - esperar a tener tokens reales de Metadata
			messageStartReceived = true

		case *types.ConverseStreamOutputMemberContentBlockStart:
			// Enviar evento content_block_start
			fmt.Fprintf(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			flusher.Flush()

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			// Extraer texto del delta
			if e.Value.Delta != nil {
				if textDelta, ok := e.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
					rawText := textDelta.Value
					
					// Procesar el texto a través del buffer XML
					processedText := xmlBuffer.ProcessChunk(rawText)
					
					// Solo enviar si hay texto procesado
					if len(processedText) > 0 {
						textJSON, err := json.Marshal(processedText)
						if err != nil {
							continue
						}
						fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", string(textJSON))
						flusher.Flush()
					}
				}
			}

		case *types.ConverseStreamOutputMemberContentBlockStop:
			// Enviar cualquier contenido restante en el buffer
			if xmlBuffer.HasBufferedContent() {
				remainingText := xmlBuffer.Flush()
				if len(remainingText) > 0 {
					textJSON, err := json.Marshal(remainingText)
					if err == nil {
						fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%s}}\n\n", string(textJSON))
						flusher.Flush()
					}
				}
			}
			
			// Enviar evento content_block_stop
			fmt.Fprintf(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
			flusher.Flush()

		case *types.ConverseStreamOutputMemberMetadata:
			// Capturar tokens finales desde metadata
			usage := e.Value.Usage
			if usage != nil {
				// Extraer tokens de uso
				if usage.InputTokens != nil {
					inputTokens = *usage.InputTokens
				}
				if usage.OutputTokens != nil {
					outputTokens = *usage.OutputTokens
				}
				if usage.CacheReadInputTokens != nil {
					cacheReadTokens = *usage.CacheReadInputTokens
				}
				if usage.CacheWriteInputTokens != nil {
					cacheWriteTokens = *usage.CacheWriteInputTokens
				}
				
				// AHORA enviar message_start con tokens REALES (buffering selectivo)
				if messageStartReceived && !messageStartSent {
					fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"%s\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d,\"cache_creation_input_tokens\":%d,\"cache_read_input_tokens\":%d}}}\n\n", 
						modelID, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens)
					flusher.Flush()
					messageStartSent = true
				}
				
				// Enviar evento ping con tokens reales para MetricsCapture (backup)
				fmt.Fprintf(w, "event: ping\ndata: {\"type\":\"ping\",\"usage\":{\"input_tokens\":%d,\"output_tokens\":%d,\"cache_creation_input_tokens\":%d,\"cache_read_input_tokens\":%d}}\n\n",
					inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens)
				flusher.Flush()
			}
			// NO enviar message_stop aquí - se envía en MessageStop

		case *types.ConverseStreamOutputMemberMessageStop:
			// Enviar evento message_delta con stop_reason y tokens finales (formato Anthropic)
			stopReason := "end_turn"
			if e.Value.StopReason != "" {
				stopReason = string(e.Value.StopReason)
			}
			fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"%s\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":%d}}\n\n", stopReason, outputTokens)
			flusher.Flush()
			
			// Enviar evento message_stop SIN usage (formato Anthropic)
			fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			flusher.Flush()
		}
	}

	// Verificar errores del stream
	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

func (this *BedrockClient) HandleProxy(w http.ResponseWriter, r *http.Request) {
	// Crear contexto de request con timing
	requestID := uuid.New().String()
	reqCtx := NewRequestContext(requestID)
	startTime := reqCtx.StartTime
	
	// Crear contexto con trazabilidad
	ctx := r.Context()
	ctx = amslog.WithRequestID(ctx, requestID)
	
	// Generar o extraer trace ID
	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
	}
	ctx = amslog.WithTraceID(ctx, traceID)
	
	// Propagar contexto al request
	r = r.WithContext(ctx)
	
	// Log de inicio con nuevo logger
	Logger.InfoContext(ctx, amslog.Event{
		Name:    EventProxyRequestStart,
		Message: "Request received",
		Fields: map[string]interface{}{
			"http.request.method": r.Method,
			"url.path":            r.URL.Path,
			"host.name":           r.Host,
		},
	})
	
	// Obtener usuario del contexto (si está autenticado)
	var user *auth.UserContext
	if u, err := auth.GetUserFromContext(ctx); err == nil {
		user = u
		Logger.InfoContext(ctx, amslog.Event{
			Name:    EventAuthJWTValidate,
			Message: "User authenticated",
			Fields: map[string]interface{}{
				"user.id":   user.UserID,
				"user.team": user.Team,
			},
		})
	}
	
	// Obtener modelo del request (para métricas)
	modelID := "unknown"
	if user != nil && user.DefaultInferenceProfile != "" {
		modelID = user.DefaultInferenceProfile
	}
	
	// Validar que el usuario tiene inference profile
	if user == nil || user.DefaultInferenceProfile == "" {
		Logger.ErrorContext(ctx, amslog.Event{
			Name:    EventProxyRequestError,
			Message: "User missing inference profile",
			Outcome: amslog.OutcomeFailure,
			Error: &amslog.ErrorInfo{
				Type:    "ValidationError",
				Message: "User must have default_inference_profile configured in JWT",
				Code:    "NO_INFERENCE_PROFILE",
			},
		})
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "User must have default_inference_profile configured in JWT"}`, http.StatusForbidden)
		return
	}
	
	// FASE 0: Leer el body original ANTES de SignRequest para preservar tools
	originalBodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()
	// Restaurar el body para SignRequest
	r.Body = io.NopCloser(bytes.NewBuffer(originalBodyBytes))
	
	// FASE 1: Firma de request usando el ARN del usuario
	endPhase := reqCtx.StartPhase("sign_request")
	cloneReq, isStream, err := this.SignRequest(r, user.DefaultInferenceProfile)
	endPhase()
	
	if err != nil {
		Logger.ErrorContext(ctx, amslog.Event{
			Name:       EventProxyRequestError,
			Message:    "Request signing failed",
			Outcome:    amslog.OutcomeFailure,
			DurationMs: reqCtx.PhaseTimings["sign_request"].Milliseconds(),
			Error: &amslog.ErrorInfo{
				Type:    "SigningError",
				Message: err.Error(),
				Code:    "SIGN_REQUEST_FAILED",
			},
		})
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	
	Logger.InfoContext(ctx, amslog.Event{
		Name:       "BEDROCK_SIGN_REQUEST",
		Message:    "Request signed successfully",
		Outcome:    amslog.OutcomeSuccess,
		DurationMs: reqCtx.PhaseTimings["sign_request"].Milliseconds(),
		Fields: map[string]interface{}{
			"is_stream":         isStream,
			"inference_profile": user.DefaultInferenceProfile,
		},
	})

	if isStream {
		// FASE 2: Parsear request body ORIGINAL para extraer system, messages y tools
		endPhase = reqCtx.StartPhase("parse_request")
		
		// Usar el body original que guardamos antes de SignRequest
		var payload map[string]interface{}
		if err := json.Unmarshal(originalBodyBytes, &payload); err != nil {
			Logger.ErrorContext(ctx, amslog.Event{
				Name:       EventProxyRequestError,
				Message:    "Failed to parse request body",
				Outcome:    amslog.OutcomeFailure,
				DurationMs: reqCtx.PhaseTimings["parse_request"].Milliseconds(),
				Error: &amslog.ErrorInfo{
					Type:    "ParseError",
					Message: err.Error(),
					Code:    "INVALID_JSON",
				},
			})
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, fmt.Sprintf(`{"error": "Failed to parse request: %s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		
		// Extraer tools y convertirlas a texto PRIMERO (para añadir al system prompt)
		var toolConfig *types.ToolConfiguration
		var toolChoice types.ToolChoice
		var toolsTextForSystemPrompt string
		
		if tools, ok := payload["tools"].([]interface{}); ok {
			// Convertir tools a JSON estructurado para añadir al system prompt
			var convErr error
			toolsTextForSystemPrompt, convErr = convertAnthropicToolsToJSON(tools)
			
			if convErr != nil {
				Logger.ErrorContext(ctx, amslog.Event{
					Name:    EventProxyRequestError,
					Message: "Failed to convert tools to JSON",
					Outcome: amslog.OutcomeFailure,
					Error: &amslog.ErrorInfo{
						Type:    "ToolConversionError",
						Message: convErr.Error(),
						Code:    "TOOL_JSON_CONVERSION_FAILED",
					},
				})
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error": "Failed to convert tools to JSON: %s"}`, convErr.Error()), http.StatusBadRequest)
				return
			}
			
			if toolsTextForSystemPrompt != "" {
				Logger.InfoContext(ctx, amslog.Event{
					Name:    "BEDROCK_TOOLS_TO_JSON",
					Message: "Tools converted to JSON for system prompt",
					Fields: map[string]interface{}{
						"tools_count": len(tools),
						"json_length": len(toolsTextForSystemPrompt),
					},
				})
			}
			
			// También convertir a ToolConfiguration (aunque NO se enviará a Bedrock)
			toolConfig, err = convertAnthropicToolsToBedrock(tools)
			if err != nil {
				Logger.ErrorContext(ctx, amslog.Event{
					Name:    EventProxyRequestError,
					Message: "Failed to convert tools",
					Outcome: amslog.OutcomeFailure,
					Error: &amslog.ErrorInfo{
						Type:    "ToolConversionError",
						Message: err.Error(),
						Code:    "TOOL_CONVERSION_FAILED",
					},
				})
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error": "Failed to convert tools: %s"}`, err.Error()), http.StatusBadRequest)
				return
			}
		}
		
		// Extraer system blocks (puede ser string o array de bloques)
		var systemBlocks []types.SystemContentBlock
		if sys, ok := payload["system"].(string); ok {
			// System es un string simple (legacy) - añadir tools al final
			systemText := sys
			if toolsTextForSystemPrompt != "" {
				systemText = sys + toolsTextForSystemPrompt
			}
			systemBlocks = []types.SystemContentBlock{
				&types.SystemContentBlockMemberText{
					Value: systemText,
				},
			}
			Logger.DebugContext(ctx, amslog.Event{
				Name:    "BEDROCK_PARSE_SYSTEM",
				Message: "Using legacy string format for system",
				Fields: map[string]interface{}{
					"tools_added": toolsTextForSystemPrompt != "",
				},
			})
		} else if sysArray, ok := payload["system"].([]interface{}); ok {
			// System es un array de bloques (con posible cache_control)
			systemBlocks = convertSystemBlocksWithCache(sysArray)
			
			// Añadir tools al final del system prompt si existen
			if toolsTextForSystemPrompt != "" {
				systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberText{
					Value: toolsTextForSystemPrompt,
				})
			}
			
			Logger.InfoContext(ctx, amslog.Event{
				Name:    "BEDROCK_PARSE_SYSTEM",
				Message: "Converted system blocks with cache support",
				Fields: map[string]interface{}{
					"system_blocks_count": len(systemBlocks),
					"tools_added":         toolsTextForSystemPrompt != "",
				},
			})
		}
		
		// LOG solo en modo DEBUG
		if this.config.DEBUG && len(systemBlocks) > 0 {
			Logger.DebugContext(ctx, amslog.Event{
				Name:    "BEDROCK_SYSTEM_PROMPT_DETAIL",
				Message: "System prompt content being sent to Bedrock",
				Fields: map[string]interface{}{
					"total_blocks": len(systemBlocks),
				},
			})
		}
		
		// Extraer max_tokens (prioridad: config > payload > default)
		maxTokens := int32(DefaultMaxTokens)
		if this.config.MaxTokens > 0 {
			maxTokens = int32(this.config.MaxTokens)
			Logger.DebugContext(ctx, amslog.Event{
				Name:    "BEDROCK_CONFIG_MAX_TOKENS",
				Message: "Using max_tokens from config",
				Fields: map[string]interface{}{
					"max_tokens": maxTokens,
				},
			})
		} else if mt, ok := payload["max_tokens"].(float64); ok {
			maxTokens = int32(mt)
			Logger.DebugContext(ctx, amslog.Event{
				Name:    "BEDROCK_REQUEST_MAX_TOKENS",
				Message: "Using max_tokens from request",
				Fields: map[string]interface{}{
					"max_tokens": maxTokens,
				},
			})
		}

		// Respetar SIEMPRE el tool_choice que envía el cliente (principio de transparencia)
		if tc, ok := payload["tool_choice"]; ok {
			toolChoice = convertAnthropicToolChoiceToBedrock(tc)
			if toolChoice != nil && toolConfig != nil {
				// Aplicar el ToolChoice del cliente al ToolConfiguration
				toolConfig.ToolChoice = toolChoice
				if this.config.DEBUG {
					Logger.DebugContext(ctx, amslog.Event{
						Name:    "BEDROCK_TOOL_CHOICE_APPLIED",
						Message: "Tool choice from client applied to ToolConfiguration",
					})
				}
			}
		} else {
			// Si no se especifica, usar 'auto' por defecto
			if toolConfig != nil {
				toolConfig.ToolChoice = &types.ToolChoiceMemberAuto{
					Value: types.AutoToolChoice{},
				}
			}
		}

		// Extraer y convertir messages
		var bedrockMessages []types.Message
		if messages, ok := payload["messages"].([]interface{}); ok {
			// Validar límite de mensajes
			if len(messages) > MaxMessagesPerRequest {
				Logger.ErrorContext(ctx, amslog.Event{
					Name:    EventProxyRequestError,
					Message: "Too many messages in request",
					Outcome: amslog.OutcomeFailure,
					Error: &amslog.ErrorInfo{
						Type:    "ValidationError",
						Message: fmt.Sprintf("Too many messages: %d (max: %d)", len(messages), MaxMessagesPerRequest),
						Code:    "TOO_MANY_MESSAGES",
					},
					Fields: map[string]interface{}{
						"message_count": len(messages),
						"max_allowed":   MaxMessagesPerRequest,
					},
				})
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error": "Too many messages: %d (max: %d)"}`, 
					len(messages), MaxMessagesPerRequest), http.StatusBadRequest)
				return
			}
			
			bedrockMessages, err = convertAnthropicToBedrockMessages(messages)
			if err != nil {
				Logger.ErrorContext(ctx, amslog.Event{
					Name:       EventProxyRequestError,
					Message:    "Failed to convert messages",
					Outcome:    amslog.OutcomeFailure,
					DurationMs: reqCtx.PhaseTimings["parse_request"].Milliseconds(),
					Error: &amslog.ErrorInfo{
						Type:    "ConversionError",
						Message: err.Error(),
						Code:    "MESSAGE_CONVERSION_FAILED",
					},
				})
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, fmt.Sprintf(`{"error": "Failed to convert messages: %s"}`, err.Error()), http.StatusBadRequest)
				return
			}
		}
		
		endPhase()
		Logger.InfoContext(ctx, amslog.Event{
			Name:       "BEDROCK_PARSE_COMPLETE",
			Message:    "Request parsing completed",
			Outcome:    amslog.OutcomeSuccess,
			DurationMs: reqCtx.PhaseTimings["parse_request"].Milliseconds(),
			Fields: map[string]interface{}{
				"messages_count":      len(bedrockMessages),
				"system_blocks_count": len(systemBlocks),
				"max_tokens":          maxTokens,
			},
		})

		// FASE 3: Streaming con Converse API
		endPhase = reqCtx.StartPhase("streaming")
		
		// Crear wrapper para capturar métricas (si hay BD y usuario)
		var metricsCapture *MetricsCapture
		var finalWriter http.ResponseWriter = w
		
		if this.db != nil && this.metricsWorker != nil && user != nil {
			metricsCapture = NewMetricsCapture(w, modelID, requestID, r)
			finalWriter = metricsCapture
		}

		// Usar Converse API directamente con system blocks
		if err := this.handleBedrockStreamConverse(finalWriter, this.client, modelID, systemBlocks, bedrockMessages, maxTokens, toolConfig, toolChoice); err != nil {
			Logger.ErrorContext(ctx, amslog.Event{
				Name:       EventBedrockError,
				Message:    "Streaming failed",
				Outcome:    amslog.OutcomeFailure,
				DurationMs: reqCtx.PhaseTimings["streaming"].Milliseconds(),
				Error: &amslog.ErrorInfo{
					Type:    "StreamingError",
					Message: err.Error(),
					Code:    "BEDROCK_STREAM_FAILED",
				},
			})
		}
		
		endPhase()
		
		Logger.InfoContext(ctx, amslog.Event{
			Name:       EventBedrockStreamComplete,
			Message:    "Streaming completed",
			Outcome:    amslog.OutcomeSuccess,
			DurationMs: reqCtx.PhaseTimings["streaming"].Milliseconds(),
		})
		
		// POST-PROCESSING: Procesar métricas en goroutine (si hay captura)
		if metricsCapture != nil && user != nil {
			go func() {
				endPhase := reqCtx.StartPhase("post_processing")
				this.processMetrics(context.Background(), user, metricsCapture, startTime)
				endPhase()
				
				Logger.InfoContext(ctx, amslog.Event{
					Name:       "METRICS_POST_PROCESS",
					Message:    "Metrics post-processing completed",
					Outcome:    amslog.OutcomeSuccess,
					DurationMs: reqCtx.PhaseTimings["post_processing"].Milliseconds(),
				})
				
				// Log final con resumen
				reqCtx.LogSummary()
				Logger.InfoContext(ctx, amslog.Event{
					Name:       EventProxyRequestEnd,
					Message:    "Request completed successfully",
					Outcome:    amslog.OutcomeSuccess,
					DurationMs: reqCtx.GetTotalDuration().Milliseconds(),
					Fields: map[string]interface{}{
						"user.id": user.UserID,
					},
				})
			}()
		} else {
			reqCtx.LogSummary()
			Logger.InfoContext(ctx, amslog.Event{
				Name:       EventProxyRequestEnd,
				Message:    "Request completed successfully",
				Outcome:    amslog.OutcomeSuccess,
				DurationMs: reqCtx.GetTotalDuration().Milliseconds(),
			})
		}
		
		return
	}

	// FASE 2: Llamada HTTP a Bedrock (no-stream)
	endPhase = reqCtx.StartPhase("bedrock_call")
	
	httpClient := http.DefaultClient
	if this.config.DEBUG {
		httpClient = &http.Client{
			Transport: loggingRoundTripper{
				wrapped: http.DefaultTransport,
			},
		}
	}

	resp, err := httpClient.Do(cloneReq)
	endPhase()
	
	if err != nil {
		Logger.ErrorContext(ctx, amslog.Event{
			Name:       EventBedrockError,
			Message:    "Bedrock API call failed",
			Outcome:    amslog.OutcomeFailure,
			DurationMs: reqCtx.PhaseTimings["bedrock_call"].Milliseconds(),
			Error: &amslog.ErrorInfo{
				Type:    "BedrockAPIError",
				Message: err.Error(),
				Code:    "BEDROCK_CALL_FAILED",
			},
		})
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	
	Logger.InfoContext(ctx, amslog.Event{
		Name:       EventBedrockInvoke,
		Message:    "Bedrock API call completed",
		Outcome:    amslog.OutcomeSuccess,
		DurationMs: reqCtx.PhaseTimings["bedrock_call"].Milliseconds(),
		Fields: map[string]interface{}{
			"http.response.status_code": resp.StatusCode,
		},
	})

	// Write modified response
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		Logger.ErrorContext(ctx, amslog.Event{
			Name:    EventProxyRequestError,
			Message: "Failed to copy response body",
			Outcome: amslog.OutcomeFailure,
			Error: &amslog.ErrorInfo{
				Type:    "ResponseCopyError",
				Message: err.Error(),
				Code:    "RESPONSE_COPY_FAILED",
			},
		})
	}
	
	// Log final
	reqCtx.LogSummary()
	Logger.InfoContext(ctx, amslog.Event{
		Name:       EventProxyRequestEnd,
		Message:    "Request completed successfully",
		Outcome:    amslog.OutcomeSuccess,
		DurationMs: reqCtx.GetTotalDuration().Milliseconds(),
		Fields: map[string]interface{}{
			"http.response.status_code": resp.StatusCode,
		},
	})
}
