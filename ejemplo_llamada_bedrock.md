# Ejemplo de Llamada a Bedrock con Tools

Este documento muestra cómo se debe convertir el campo `tools` de Anthropic al formato `toolConfig` de Bedrock.

## Formato Anthropic (Entrada)

```json
{
  "model": "claude-3-5-sonnet-20241022",
  "max_tokens": 1024,
  "tools": [
    {
      "name": "get_weather",
      "description": "Get the current weather in a given location",
      "input_schema": {
        "type": "object",
        "properties": {
          "location": {
            "type": "string",
            "description": "The city and state, e.g. San Francisco, CA"
          },
          "unit": {
            "type": "string",
            "enum": ["celsius", "fahrenheit"],
            "description": "The unit of temperature"
          }
        },
        "required": ["location"]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "What's the weather in San Francisco?"
    }
  ]
}
```

## Formato Bedrock (Salida)

```go
input := &bedrockRuntime.ConverseStreamInput{
    ModelId:  &modelID,
    Messages: messages,
    System:   systemBlocks,
    ToolConfig: &types.ToolConfiguration{
        Tools: []types.Tool{
            &types.ToolMemberToolSpec{
                Value: types.ToolSpecification{
                    Name:        aws.String("get_weather"),
                    Description: aws.String("Get the current weather in a given location"),
                    InputSchema: &types.ToolInputSchemaMemberJson{
                        Value: json.RawMessage(`{
                            "type": "object",
                            "properties": {
                                "location": {
                                    "type": "string",
                                    "description": "The city and state, e.g. San Francisco, CA"
                                },
                                "unit": {
                                    "type": "string",
                                    "enum": ["celsius", "fahrenheit"],
                                    "description": "The unit of temperature"
                                }
                            },
                            "required": ["location"]
                        }`),
                    },
                },
            },
        },
    },
    InferenceConfig: &types.InferenceConfiguration{
        MaxTokens:   aws.Int32(maxTokens),
        Temperature: aws.Float32(DefaultTemperature),
    },
}
```

## Notas Importantes

1. **Conversión de input_schema**: El `input_schema` de Anthropic se convierte a `InputSchema` de Bedrock usando `ToolInputSchemaMemberJson` con `json.RawMessage`

2. **Estructura de Tools**: Cada tool en Anthropic se convierte a un `ToolMemberToolSpec` en Bedrock

3. **Campos requeridos**:
   - `name`: Nombre de la herramienta
   - `description`: Descripción de la herramienta
   - `input_schema`: Schema JSON del input

4. **Tool Choice**: Si Anthropic envía `tool_choice`, se debe convertir a `ToolChoice` de Bedrock:
   - `"auto"` → `types.ToolChoiceMemberAuto{Value: types.AutoToolChoice{}}`
   - `"any"` → `types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}}`
   - `{"type": "tool", "name": "tool_name"}` → `types.ToolChoiceMemberTool{Value: types.SpecificToolChoice{Name: aws.String("tool_name")}}`