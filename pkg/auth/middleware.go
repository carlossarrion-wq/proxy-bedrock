package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"bedrock-proxy-test/pkg/database"
)

// ContextKey es el tipo para las claves del contexto
type ContextKey string

const (
	// UserContextKey es la clave para almacenar información del usuario en el contexto
	UserContextKey ContextKey = "user"
)

// UserContext contiene la información del usuario autenticado
type UserContext struct {
	UserID                  string
	Email                   string
	IAMUsername             string
	IAMGroups               []string
	DefaultInferenceProfile string
	Team                    string
	Person                  string
	JTI                     string
}

// AuthMiddleware es el middleware de autenticación JWT
type AuthMiddleware struct {
	jwtConfig JWTConfig
	db        *database.Database
}

// NewAuthMiddleware crea una nueva instancia del middleware de autenticación
func NewAuthMiddleware(db *database.Database, jwtConfig JWTConfig) *AuthMiddleware {
	return &AuthMiddleware{
		jwtConfig: jwtConfig,
		db:        db,
	}
}

// Middleware es el handler HTTP que valida el JWT
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenString string
		var err error

		// Intentar extraer token del header Authorization (formato: Bearer <token>)
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			tokenString, err = ExtractBearerToken(authHeader)
			if err != nil {
				am.respondError(w, http.StatusUnauthorized, fmt.Sprintf("invalid authorization header: %v", err))
				return
			}
		} else {
			// Si no hay Authorization, intentar con x-api-key (formato usado por Cline)
			apiKey := r.Header.Get("x-api-key")
			if apiKey == "" {
				am.respondError(w, http.StatusUnauthorized, "missing authorization header or x-api-key")
				return
			}
			tokenString = apiKey
		}

		// Validar firma y estructura del JWT
		claims, err := ValidateToken(tokenString, am.jwtConfig.SecretKey)
		if err != nil {
			am.respondError(w, http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err))
			return
		}

		// Calcular hash del token para buscar en BD
		tokenHash := HashToken(tokenString)

		// Validar token contra base de datos
		tokenInfo, err := am.db.ValidateToken(r.Context(), tokenHash)
		if err != nil {
			am.respondError(w, http.StatusUnauthorized, fmt.Sprintf("token validation failed: %v", err))
			return
		}

		// Verificar que el token no esté revocado
		if tokenInfo.IsRevoked {
			am.respondError(w, http.StatusUnauthorized, "token has been revoked")
			return
		}

		// Verificar que el user_id del token coincida con el de los claims
		if tokenInfo.UserID != claims.UserID {
			am.respondError(w, http.StatusUnauthorized, "token user mismatch")
			return
		}

		// Crear contexto de usuario
		userCtx := UserContext{
			UserID:                  claims.UserID,
			Email:                   claims.Email,
			IAMUsername:             claims.IAMUsername,
			IAMGroups:               claims.IAMGroups,
			DefaultInferenceProfile: claims.DefaultInferenceProfile,
			Team:                    claims.Team,
			Person:                  claims.Person,
			JTI:                     claims.ID,
		}

		// Log información del JWT autenticado
		fmt.Printf("[JWT-AUTH] ✅ Token válido para usuario: %s (%s)\n", userCtx.Email, userCtx.IAMUsername)
		fmt.Printf("[JWT-AUTH]    Team: %s | Person: %s\n", userCtx.Team, userCtx.Person)
		fmt.Printf("[JWT-AUTH]    Inference Profile: %s\n", userCtx.DefaultInferenceProfile)
		fmt.Printf("[JWT-AUTH]    JTI: %s\n", userCtx.JTI)

		// Añadir información del usuario al contexto de la request
		ctx := context.WithValue(r.Context(), UserContextKey, userCtx)
		
		// Añadir inference_profile al contexto para que bedrock.go lo use
		if claims.DefaultInferenceProfile != "" {
			ctx = context.WithValue(ctx, "inference_profile", claims.DefaultInferenceProfile)
			fmt.Printf("[JWT-AUTH]    🎯 Usando Inference Profile del JWT: %s\n", claims.DefaultInferenceProfile)
		}

		// Continuar con el siguiente handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// respondError envía una respuesta de error en formato JSON
func (am *AuthMiddleware) respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error":"%s"}`, message)
}

// GetUserFromContext extrae la información del usuario del contexto
func GetUserFromContext(ctx context.Context) (*UserContext, error) {
	user, ok := ctx.Value(UserContextKey).(UserContext)
	if !ok {
		return nil, fmt.Errorf("user not found in context")
	}
	return &user, nil
}

// RequireGroups es un middleware adicional que verifica que el usuario pertenezca a grupos específicos
func RequireGroups(requiredGroups []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := GetUserFromContext(r.Context())
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error":"user not authenticated"}`)
				return
			}

			// Verificar si el usuario pertenece a alguno de los grupos requeridos
			hasGroup := false
			for _, requiredGroup := range requiredGroups {
				for _, userGroup := range user.IAMGroups {
					if strings.EqualFold(userGroup, requiredGroup) {
						hasGroup = true
						break
					}
				}
				if hasGroup {
					break
				}
			}

			if !hasGroup {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error":"insufficient permissions"}`)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
