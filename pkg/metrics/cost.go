package metrics

import (
	"fmt"
)

// ModelPricing contiene los precios por modelo de Bedrock
type ModelPricing struct {
	InputPer1KTokens  float64 // Precio por 1000 tokens de input
	OutputPer1KTokens float64 // Precio por 1000 tokens de output
}

// PricingTable contiene los precios de todos los modelos Bedrock
var PricingTable = map[string]ModelPricing{
	// Claude 3 Family
	"anthropic.claude-3-opus-20240229-v1:0": {
		InputPer1KTokens:  0.015, // $15 per 1M input tokens
		OutputPer1KTokens: 0.075, // $75 per 1M output tokens
	},
	"anthropic.claude-3-sonnet-20240229-v1:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"anthropic.claude-3-haiku-20240307-v1:0": {
		InputPer1KTokens:  0.00025, // $0.25 per 1M input tokens
		OutputPer1KTokens: 0.00125, // $1.25 per 1M output tokens
	},

	// Claude 3.5 Family
	"anthropic.claude-3-5-sonnet-20240620-v1:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"anthropic.claude-3-5-sonnet-20241022-v2:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"anthropic.claude-3-5-haiku-20241022-v1:0": {
		InputPer1KTokens:  0.001,  // $1 per 1M input tokens
		OutputPer1KTokens: 0.005,  // $5 per 1M output tokens
	},

	// Claude Sonnet 4.5 (Inference Profile)
	"us.anthropic.claude-sonnet-4-5-v2:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"eu.anthropic.claude-sonnet-4-5-v2:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"eu.anthropic.claude-sonnet-4-5-20250929-v1:0": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},

	// Application Inference Profiles (ARNs)
	// TODO FUTURO: Implementar mapeo automático basado en el nombre del Application Profile
	// Ver MEJORA_FUTURA_MAPEO_ARN_PRICING.md para detalles de la implementación propuesta
	"arn:aws:bedrock:eu-west-1:701055077130:application-inference-profile/hjy3duh3aoos": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens (Claude Sonnet 4.5)
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},
	"arn:aws:bedrock:eu-west-1:701055077130:application-inference-profile/kb2twga41cr4": {
		InputPer1KTokens:  0.003, // $3 per 1M input tokens (Claude Sonnet 4.5)
		OutputPer1KTokens: 0.015, // $15 per 1M output tokens
	},

	// Amazon Titan
	"amazon.titan-text-express-v1": {
		InputPer1KTokens:  0.0002,  // $0.20 per 1M input tokens
		OutputPer1KTokens: 0.0006,  // $0.60 per 1M output tokens
	},
	"amazon.titan-text-lite-v1": {
		InputPer1KTokens:  0.00015, // $0.15 per 1M input tokens
		OutputPer1KTokens: 0.0002,  // $0.20 per 1M output tokens
	},
	"amazon.titan-text-premier-v1:0": {
		InputPer1KTokens:  0.0005, // $0.50 per 1M input tokens
		OutputPer1KTokens: 0.0015, // $1.50 per 1M output tokens
	},

	// AI21 Labs Jurassic
	"ai21.j2-ultra-v1": {
		InputPer1KTokens:  0.0188, // $18.80 per 1M tokens
		OutputPer1KTokens: 0.0188, // $18.80 per 1M tokens
	},
	"ai21.j2-mid-v1": {
		InputPer1KTokens:  0.0125, // $12.50 per 1M tokens
		OutputPer1KTokens: 0.0125, // $12.50 per 1M tokens
	},

	// Cohere
	"cohere.command-text-v14": {
		InputPer1KTokens:  0.0015, // $1.50 per 1M tokens
		OutputPer1KTokens: 0.002,  // $2.00 per 1M tokens
	},
	"cohere.command-light-text-v14": {
		InputPer1KTokens:  0.0003, // $0.30 per 1M tokens
		OutputPer1KTokens: 0.0006, // $0.60 per 1M tokens
	},

	// Meta Llama
	"meta.llama3-8b-instruct-v1:0": {
		InputPer1KTokens:  0.0003, // $0.30 per 1M tokens
		OutputPer1KTokens: 0.0006, // $0.60 per 1M tokens
	},
	"meta.llama3-70b-instruct-v1:0": {
		InputPer1KTokens:  0.00265, // $2.65 per 1M tokens
		OutputPer1KTokens: 0.0035,  // $3.50 per 1M tokens
	},

	// Mistral AI
	"mistral.mistral-7b-instruct-v0:2": {
		InputPer1KTokens:  0.00015, // $0.15 per 1M tokens
		OutputPer1KTokens: 0.0002,  // $0.20 per 1M tokens
	},
	"mistral.mixtral-8x7b-instruct-v0:1": {
		InputPer1KTokens:  0.00045, // $0.45 per 1M tokens
		OutputPer1KTokens: 0.0007,  // $0.70 per 1M tokens
	},
	"mistral.mistral-large-2402-v1:0": {
		InputPer1KTokens:  0.008, // $8 per 1M tokens
		OutputPer1KTokens: 0.024, // $24 per 1M tokens
	},
}

// CalculateCost calcula el coste de un request basado en tokens y modelo
func CalculateCost(modelID string, inputTokens, outputTokens int64) (float64, error) {
	pricing, exists := PricingTable[modelID]
	if !exists {
		return 0, fmt.Errorf("pricing not found for model: %s", modelID)
	}

	// Calcular coste
	inputCost := (float64(inputTokens) / 1000.0) * pricing.InputPer1KTokens
	outputCost := (float64(outputTokens) / 1000.0) * pricing.OutputPer1KTokens
	totalCost := inputCost + outputCost

	return totalCost, nil
}

// GetModelPricing retorna el pricing de un modelo específico
func GetModelPricing(modelID string) (ModelPricing, error) {
	pricing, exists := PricingTable[modelID]
	if !exists {
		return ModelPricing{}, fmt.Errorf("pricing not found for model: %s", modelID)
	}
	return pricing, nil
}

// EstimateCost estima el coste antes de hacer el request (útil para validación)
func EstimateCost(modelID string, estimatedInputTokens, estimatedOutputTokens int64) (float64, error) {
	return CalculateCost(modelID, estimatedInputTokens, estimatedOutputTokens)
}

// FormatCost formatea un coste en USD con 6 decimales
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.6f", cost)
}

// CostBreakdown contiene el desglose de costes
type CostBreakdown struct {
	ModelID      string
	InputTokens  int64
	OutputTokens int64
	InputCost    float64
	OutputCost   float64
	TotalCost    float64
}

// CalculateCostBreakdown calcula el coste con desglose detallado
func CalculateCostBreakdown(modelID string, inputTokens, outputTokens int64) (*CostBreakdown, error) {
	pricing, exists := PricingTable[modelID]
	if !exists {
		return nil, fmt.Errorf("pricing not found for model: %s", modelID)
	}

	inputCost := (float64(inputTokens) / 1000.0) * pricing.InputPer1KTokens
	outputCost := (float64(outputTokens) / 1000.0) * pricing.OutputPer1KTokens

	return &CostBreakdown{
		ModelID:      modelID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		InputCost:    inputCost,
		OutputCost:   outputCost,
		TotalCost:    inputCost + outputCost,
	}, nil
}
