package pkg

import (
	"encoding/json"
	"fmt"
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

// convertAnthropicToolsToText convierte tools de formato Anthropic a texto descriptivo
// para incluir en el system prompt (igual que hace Cline)
func convertAnthropicToolsToText(anthropicTools []interface{}) string {
	if len(anthropicTools) == 0 {
		return ""
	}

	var toolsText strings.Builder
	
	toolsText.WriteString("\n\n# Tool Use Formatting\n\n")
	toolsText.WriteString("Tool use is formatted using XML-style tags. The tool name is enclosed in opening and closing tags, ")
	toolsText.WriteString("and each parameter is similarly enclosed within its own set of tags. Here's the structure:\n\n")
	toolsText.WriteString("<tool_name>\n<parameter1_name>value1</parameter1_name>\n<parameter2_name>value2</parameter2_name>\n")
	toolsText.WriteString("...\n</tool_name>\n\n")
	toolsText.WriteString("Always adhere to this format for tool use to ensure proper parsing and execution.\n\n")
	toolsText.WriteString("# Tools\n\n")

	for _, tool := range anthropicTools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)

		if name == "" {
			continue
		}

		// Escribir nombre y descripción de la tool
		toolsText.WriteString(fmt.Sprintf("## %s\n", name))
		if description != "" {
			toolsText.WriteString(fmt.Sprintf("Description: %s\n", description))
		}

		// Procesar input_schema para extraer parámetros
		if inputSchema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
			if properties, ok := inputSchema["properties"].(map[string]interface{}); ok {
				required := []string{}
				if req, ok := inputSchema["required"].([]interface{}); ok {
					for _, r := range req {
						if reqStr, ok := r.(string); ok {
							required = append(required, reqStr)
						}
					}
				}

				toolsText.WriteString("Parameters:\n")
				for paramName, paramValue := range properties {
					paramMap, ok := paramValue.(map[string]interface{})
					if !ok {
						continue
					}

					isRequired := false
					for _, req := range required {
						if req == paramName {
							isRequired = true
							break
						}
					}

					reqStr := "optional"
					if isRequired {
						reqStr = "required"
					}

					paramDesc, _ := paramMap["description"].(string)
					toolsText.WriteString(fmt.Sprintf("- %s: (%s) %s\n", paramName, reqStr, paramDesc))
				}
			}
		}

		// Ejemplo de uso
		toolsText.WriteString("Usage:\n")
		toolsText.WriteString(fmt.Sprintf("<%s>\n", name))
		
		// Añadir parámetros de ejemplo
		if inputSchema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
			if properties, ok := inputSchema["properties"].(map[string]interface{}); ok {
				for paramName := range properties {
					toolsText.WriteString(fmt.Sprintf("<%s>value here</%s>\n", paramName, paramName))
				}
			}
		}
		
		toolsText.WriteString(fmt.Sprintf("</%s>\n\n", name))
	}

	return toolsText.String()
}
