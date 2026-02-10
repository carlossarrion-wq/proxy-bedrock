package amslog

import (
	"regexp"
	"strings"
)

// Sanitizer sanitiza datos sensibles en los logs
type Sanitizer struct {
	sensitiveKeys map[string]bool
	emailRegex    *regexp.Regexp
	dniRegex      *regexp.Regexp
}

// NewSanitizer crea un nuevo sanitizador
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		sensitiveKeys: map[string]bool{
			"password":       true,
			"passwd":         true,
			"pwd":            true,
			"token":          true,
			"access_token":   true,
			"refresh_token":  true,
			"secret":         true,
			"api_key":        true,
			"apikey":         true,
			"authorization":  true,
			"auth":           true,
			"bearer":         true,
		},
		emailRegex: regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
		dniRegex:   regexp.MustCompile(`\d{8}[A-Z]`),
	}
}

// Sanitize sanitiza un mapa de datos
func (s *Sanitizer) Sanitize(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range data {
		result[key] = s.sanitizeValue(key, value)
	}
	return result
}

// sanitizeValue sanitiza un valor según su clave y tipo
func (s *Sanitizer) sanitizeValue(key string, value interface{}) interface{} {
	// Verificar si la clave es sensible
	lowerKey := strings.ToLower(key)
	if s.sensitiveKeys[lowerKey] {
		return "***REDACTED***"
	}

	// Sanitizar según el tipo
	switch v := value.(type) {
	case string:
		return s.sanitizeString(v)
	case map[string]interface{}:
		return s.Sanitize(v)
	case []interface{}:
		return s.sanitizeSlice(v)
	default:
		return value
	}
}

// sanitizeString sanitiza un string
func (s *Sanitizer) sanitizeString(value string) string {
	// Enmascarar emails
	if s.emailRegex.MatchString(value) {
		return s.maskEmail(value)
	}

	// Enmascarar DNI/NIE
	if s.dniRegex.MatchString(value) {
		return s.maskDNI(value)
	}

	return value
}

// sanitizeSlice sanitiza un slice
func (s *Sanitizer) sanitizeSlice(slice []interface{}) []interface{} {
	result := make([]interface{}, len(slice))
	for i, item := range slice {
		switch v := item.(type) {
		case string:
			result[i] = s.sanitizeString(v)
		case map[string]interface{}:
			result[i] = s.Sanitize(v)
		case []interface{}:
			result[i] = s.sanitizeSlice(v)
		default:
			result[i] = item
		}
	}
	return result
}

// maskEmail enmascara un email
func (s *Sanitizer) maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	username := parts[0]
	domain := parts[1]

	if len(username) <= 1 {
		return username + "***@" + domain
	}

	return string(username[0]) + "***@" + domain
}

// maskDNI enmascara un DNI
func (s *Sanitizer) maskDNI(dni string) string {
	if len(dni) < 4 {
		return "***"
	}
	return "***" + dni[len(dni)-4:]
}