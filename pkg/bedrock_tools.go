package pkg

import (
	"encoding/json"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// convertAnthropicToolsToBedrock convierte tools de formato Anthropic a formato Bedrock ToolConfiguration
func convertAnthropicToolsToBedrock(anthropicTools []interface{}) (*types.ToolConfiguration, error) {
	if len(anthropicTools) == 0 {
		return nil, nil
	}

	var bedrockTools []types.Tool

	for _, tool := range anthropicTools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		// Extraer campos básicos
		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)

		if name == "" {
			Log.Warningf("[TOOLS_CONVERSION] Skipping tool without name")
			continue
		}
		
		// Bedrock requiere que description no esté vacía
		if description == "" {
			description = name // Usar el nombre como descripción si está vacía
			Log.Warningf("[TOOLS_CONVERSION] Tool %s has empty description, using name as description", name)
		}

		// Extraer input_schema
		var inputSchemaJSON json.RawMessage
		if inputSchema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
			// Convertir el map a JSON
			schemaBytes, err := json.Marshal(inputSchema)
			if err != nil {
				Log.Errorf("[TOOLS_CONVERSION] Failed to marshal input_schema for tool %s: %v", name, err)
				continue
			}
			inputSchemaJSON = json.RawMessage(schemaBytes)
		}

		// Crear ToolSpecification de Bedrock
		toolSpec := types.ToolSpecification{
			Name:        aws.String(name),
			Description: aws.String(description),
		}

		// InputSchema es OBLIGATORIO en Bedrock
		// NewLazyDocument acepta interface{} (cualquier tipo Go), NO JSON raw
		var schemaInterface interface{}
		if len(inputSchemaJSON) > 0 {
			// Parsear el JSON a un map[string]interface{}
			var schemaMap map[string]interface{}
			if err := json.Unmarshal(inputSchemaJSON, &schemaMap); err != nil {
				Log.Errorf("[TOOLS_CONVERSION] Failed to parse input_schema for tool %s: %v", name, err)
				// Usar schema vacío si falla
				schemaInterface = map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{},
				}
			} else {
				schemaInterface = schemaMap
			}
		} else {
			// Schema vacío pero válido (JSON Schema básico)
			schemaInterface = map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{},
			}
		}
		
		// Pasar el map directamente a NewLazyDocument
		toolSpec.InputSchema = &types.ToolInputSchemaMemberJson{
			Value: document.NewLazyDocument(schemaInterface),
		}

		// Añadir tool a la lista
		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: toolSpec,
		})

		Log.Infof("[TOOLS_CONVERSION] Converted tool: %s (description: %s, has_schema: %v)",
			name, description, len(inputSchemaJSON) > 0)
	}

	if len(bedrockTools) == 0 {
		return nil, nil
	}

	// Crear ToolConfiguration SIN ToolChoice por defecto
	// El ToolChoice se añadirá desde bedrock.go respetando lo que envía el cliente
	toolConfig := &types.ToolConfiguration{
		Tools: bedrockTools,
	}

	return toolConfig, nil
}

// convertAnthropicToolChoiceToBedrock convierte tool_choice de Anthropic a Bedrock
func convertAnthropicToolChoiceToBedrock(toolChoice interface{}) types.ToolChoice {
	if toolChoice == nil {
		return nil
	}

	// Si es un string simple
	if choiceStr, ok := toolChoice.(string); ok {
		switch choiceStr {
		case "auto":
			return &types.ToolChoiceMemberAuto{
				Value: types.AutoToolChoice{},
			}
		case "any":
			return &types.ToolChoiceMemberAny{
				Value: types.AnyToolChoice{},
			}
		}
	}

	// Si es un objeto con type
	if choiceMap, ok := toolChoice.(map[string]interface{}); ok {
		if choiceType, ok := choiceMap["type"].(string); ok {
			switch choiceType {
			case "auto":
				return &types.ToolChoiceMemberAuto{
					Value: types.AutoToolChoice{},
				}
			case "any":
				return &types.ToolChoiceMemberAny{
					Value: types.AnyToolChoice{},
				}
			case "tool":
				// Tool específica
				if toolName, ok := choiceMap["name"].(string); ok {
					return &types.ToolChoiceMemberTool{
						Value: types.SpecificToolChoice{
							Name: aws.String(toolName),
						},
					}
				}
			}
		}
	}

	return nil
}

// convertAnthropicToolsToJSON convierte tools de formato Anthropic a formato XML
// compatible con Cline (que espera sintaxis XML para tool calls)
func convertAnthropicToolsToJSON(anthropicTools []interface{}) (string, error) {
	if len(anthropicTools) == 0 {
		return "", nil
	}

	var result strings.Builder
	result.WriteString("\n\n# Available MCP Tools\n\n")
	result.WriteString("The following tools are available as MCP (Model Context Protocol) tools. ")
	result.WriteString("Each tool has a name, description, and input schema that defines its parameters.\n\n")
	result.WriteString("```json\n")
	result.WriteString("[\n")
	
	for i, tool := range anthropicTools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)
		inputSchema, _ := toolMap["input_schema"].(map[string]interface{})

		if name == "" {
			continue
		}

		// Serializar cada tool como JSON
		toolJSON := map[string]interface{}{
			"name":         name,
			"description":  description,
			"input_schema": inputSchema,
		}
		
		toolBytes, err := json.MarshalIndent(toolJSON, "  ", "  ")
		if err != nil {
			continue
		}
		
		if i > 0 {
			result.WriteString(",\n")
		}
		result.WriteString("  ")
		result.WriteString(string(toolBytes))
	}
	
	result.WriteString("\n]\n```\n\n")
	result.WriteString("To use a tool, reference it by name and provide parameters according to its input schema.\n")

	return result.String(), nil
}
