## How to Use Tools

To use a tool, you MUST use XML tags in this EXACT format:

<tool_name>
<parameter_name>parameter_value</parameter_name>
<parameter_name>parameter_value</parameter_name>
<task_progress>- [ ] Item 1
- [ ] Item 2</task_progress>
</tool_name>

CRITICAL RULES:
1. Use the tool name as the opening and closing XML tag (e.g., <execute_command>, <read_file>)
2. Each parameter must be wrapped in its own XML tag with the parameter name
3. The task_progress parameter is REQUIRED and must be included as a separate XML tag
4. Do NOT use <invoke>, <function_calls>, <tool_use>, or any wrapper tags
5. Do NOT use JSON format - only XML tags are accepted

Example of CORRECT tool usage:
<execute_command>
<command>curl -L https://example.com</command>
<requires_approval>false</requires_approval>
<task_progress>- [ ] Fetch webpage
- [ ] Analyze content</task_progress>
</execute_command>

Example of INCORRECT tool usage (DO NOT DO THIS):
<function_calls>
<invoke name="execute_command">
<parameter name="command">curl -L https://example.com</parameter>
<parameter name="requires_approval">false</parameter>
</invoke>
</function_calls>

Another INCORRECT example (DO NOT DO THIS):
{
  "name": "execute_command",
  "input": {
    "command": "curl -L https://example.com"
  }
}

Remember: Use the tool name directly as the XML tag, with each parameter as a nested XML tag.
