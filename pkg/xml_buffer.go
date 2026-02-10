package pkg

import (
	"strings"
)

// XMLTagBuffer maneja el buffering de tags XML para evitar que se corten entre chunks
type XMLTagBuffer struct {
	buffer        string
	maxBufferSize int
}

// NewXMLTagBuffer crea un nuevo buffer para tags XML con el tamaño máximo especificado
func NewXMLTagBuffer(maxBufferSize int) *XMLTagBuffer {
	return &XMLTagBuffer{
		buffer:        "",
		maxBufferSize: maxBufferSize,
	}
}

// getLastNChars retorna los últimos N caracteres de un string, escapando caracteres especiales
func getLastNChars(s string, n int) string {
	if len(s) <= n {
		return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\\n"), "\r", "\\r")
	}
	return strings.ReplaceAll(strings.ReplaceAll(s[len(s)-n:], "\n", "\\n"), "\r", "\\r")
}

// ProcessChunk procesa un chunk de texto, asegurando que los tags XML no se corten
// Retorna el texto listo para enviar y retiene cualquier tag incompleto en el buffer
func (b *XMLTagBuffer) ProcessChunk(chunk string) string {
	// Combinar buffer anterior con nuevo chunk
	fullText := b.buffer + chunk
	
	// Si el texto está vacío, no hay nada que procesar
	if len(fullText) == 0 {
		b.buffer = ""
		return ""
	}
	
	// Buscar el último '<' en el texto
	lastOpenBracket := strings.LastIndex(fullText, "<")
	
	// Si no hay '<', enviar todo
	if lastOpenBracket == -1 {
		b.buffer = ""
		Log.Debugf("[XML_BUFFER_DEBUG] 🟢 NO_BRACKET - Sending all (%d chars), last 10: '%s'", 
			len(fullText), getLastNChars(fullText, 10))
		return fullText
	}
	
	// Verificar si hay un '>' después del último '<'
	textAfterBracket := fullText[lastOpenBracket:]
	closeBracketPos := strings.Index(textAfterBracket, ">")
	
	// Si hay un '>' después del '<', el tag está completo
	if closeBracketPos != -1 {
		// Tag completo, enviar todo
		b.buffer = ""
		Log.Debugf("[XML_BUFFER_DEBUG] 🟢 COMPLETE_TAG - Sending all (%d chars), last 10: '%s'", 
			len(fullText), getLastNChars(fullText, 10))
		return fullText
	}
	
	// El tag está incompleto, retener desde el último '<'
	// Pero solo si está cerca del final (configurado por maxBufferSize)
	// Y solo si parece ser un tag XML (empieza con letra, / o !)
	distanceFromEnd := len(fullText) - lastOpenBracket
	
	if distanceFromEnd <= b.maxBufferSize {
		// Verificar si parece un tag XML
		if len(textAfterBracket) > 1 {
			firstChar := textAfterBracket[1]
			isLikelyTag := (firstChar >= 'a' && firstChar <= 'z') ||
				(firstChar >= 'A' && firstChar <= 'Z') ||
				firstChar == '/' || firstChar == '!' || firstChar == '_'
			
			if isLikelyTag {
				// Retener el posible tag incompleto
				toSend := fullText[:lastOpenBracket]
				b.buffer = fullText[lastOpenBracket:]
				
				Log.Debugf("[XML_BUFFER_DEBUG] 🟡 INCOMPLETE_TAG - Sending (%d chars), last 10: '%s' | Buffering (%d chars): '%s'", 
					len(toSend), getLastNChars(toSend, 10), 
					len(b.buffer), getLastNChars(b.buffer, 20))
				
				return toSend
			}
		} else {
			// Solo hay '<' al final, retenerlo por si acaso
			toSend := fullText[:lastOpenBracket]
			b.buffer = fullText[lastOpenBracket:]
			
			Log.Debugf("[XML_BUFFER_DEBUG] 🟡 LONE_BRACKET - Sending (%d chars), last 10: '%s' | Buffering: '<'", 
				len(toSend), getLastNChars(toSend, 10))
			
			return toSend
		}
	}
	
	// Si el '<' está muy lejos del final o no parece un tag,
	// probablemente es parte del contenido, enviar todo
	b.buffer = ""
	Log.Debugf("[XML_BUFFER_DEBUG] 🟢 TOO_FAR - Sending all (%d chars), last 10: '%s' | Distance from end: %d", 
		len(fullText), getLastNChars(fullText, 10), distanceFromEnd)
	return fullText
}

// Flush retorna cualquier contenido restante en el buffer
func (b *XMLTagBuffer) Flush() string {
	remaining := b.buffer
	b.buffer = ""
	
	if len(remaining) > 0 {
		Log.Debugf("[XML_BUFFER_DEBUG] 🔵 FLUSH - Sending buffered content (%d chars): '%s'", 
			len(remaining), getLastNChars(remaining, 30))
	}
	
	return remaining
}

// HasBufferedContent indica si hay contenido en el buffer
func (b *XMLTagBuffer) HasBufferedContent() bool {
	return len(b.buffer) > 0
}