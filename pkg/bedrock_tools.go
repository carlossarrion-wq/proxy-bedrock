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
	result.WriteString("## How to Use Tools\n\n")
	result.WriteString("To use a tool, you MUST use XML tags in this EXACT format:\n\n")
	result.WriteString("<tool_name>\n")
	result.WriteString("<parameter_name>parameter_value</parameter_name>\n")
	result.WriteString("<parameter_name>parameter_value</parameter_name>\n")
	result.WriteString("<task_progress>- [ ] Item 1\n- [ ] Item 2</task_progress>\n")
	result.WriteString("</tool_name>\n\n")
	result.WriteString("CRITICAL RULES:\n")
	result.WriteString("1. Use the tool name as the opening and closing XML tag (e.g., <execute_command>, <read_file>)\n")
	result.WriteString("2. Each parameter must be wrapped in its own XML tag with the parameter name\n")
	result.WriteString("3. The task_progress parameter is REQUIRED and must be included as a separate XML tag\n")
	result.WriteString("4. Do NOT use <invoke>, <function_calls>, <tool_use>, or any wrapper tags\n")
	result.WriteString("5. Do NOT use JSON format - only XML tags are accepted\n\n")
	result.WriteString("Example of CORRECT tool usage:\n")
	result.WriteString("<execute_command>\n")
	result.WriteString("<command>curl -L https://example.com</command>\n")
	result.WriteString("<requires_approval>false</requires_approval>\n")
	result.WriteString("<task_progress>- [ ] Fetch webpage\n- [ ] Analyze content</task_progress>\n")
	result.WriteString("</execute_command>\n\n")
	result.WriteString("Example of INCORRECT tool usage (DO NOT DO THIS):\n")
	result.WriteString("<invoke name=\"read_file\">\n")
	result.WriteString("<parameter name=\"path\">file.txt</parameter>\n")
	result.WriteString("<parameter name=\"task_progress\">- [ ] Read file</parameter>\n")
	result.WriteString("</invoke>\n\n")
	result.WriteString("Remember: Use the tool name directly as the XML tag, with each parameter as a nested XML tag.\n")

	return result.String(), nil
}
