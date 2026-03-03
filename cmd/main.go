package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"bedrock-proxy-test/pkg"
	"bedrock-proxy-test/pkg/amslog"
	"bedrock-proxy-test/pkg/auth"
	"bedrock-proxy-test/pkg/database"
	"bedrock-proxy-test/pkg/metrics"
	"bedrock-proxy-test/pkg/scheduler"
)

// chainMiddlewares aplica middlewares en orden a un handler
func chainMiddlewares(handler http.HandlerFunc, mws ...func(http.Handler) http.Handler) http.HandlerFunc {
	h := http.Handler(handler)
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h.ServeHTTP
}

func main() {
	// Inicializar logger según Política de Logs v1.0
	pkg.InitLogger()
	defer pkg.CloseLogger()
	
	// Log de inicio del servidor
	pkg.Logger.Info(amslog.Event{
		Name:    pkg.EventServerStart,
		Message: "Bedrock Proxy starting",
		Fields: map[string]interface{}{
			"port": os.Getenv("PORT"),
		},
	})
	
	// Cargar configuración desde variables de entorno
	config := pkg.LoadBedrockConfigWithEnv()
	
	// Verificar que las credenciales estén configuradas
	if config.AccessKey == "" || config.SecretKey == "" || config.Region == "" {
		fmt.Println("Error: Faltan credenciales de AWS. Configura las siguientes variables de entorno:")
		fmt.Println("  AWS_BEDROCK_ACCESS_KEY")
		fmt.Println("  AWS_BEDROCK_SECRET_KEY")
		fmt.Println("  AWS_BEDROCK_REGION")
		fmt.Println("\nEjemplo:")
		fmt.Println("  export AWS_BEDROCK_ACCESS_KEY=tu_access_key")
		fmt.Println("  export AWS_BEDROCK_SECRET_KEY=tu_secret_key")
		fmt.Println("  export AWS_BEDROCK_REGION=us-east-1")
		os.Exit(1)
	}
	
	// Inicializar conexión a PostgreSQL (opcional)
	var db *database.Database
	fmt.Println("🔌 Intentando conectar a PostgreSQL...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var err error
	db, err = pkg.InitializeDatabase(ctx)
	if err != nil {
		fmt.Printf("⚠️  Error conectando a PostgreSQL: %v\n", err)
		fmt.Println("⚠️  El proxy continuará sin funcionalidades de BD (sin auth/cuotas)")
		db = nil
	} else {
		// Verificar conectividad
		if err := db.Ping(ctx); err != nil {
			fmt.Printf("⚠️  PostgreSQL ping falló: %v\n", err)
			fmt.Println("⚠️  El proxy continuará sin funcionalidades de BD")
			db.Close()
			db = nil
		} else {
			fmt.Println("✅ PostgreSQL conectado exitosamente")
			stats := db.Stats()
			fmt.Printf("📊 Pool de conexiones - Total: %d, Idle: %d, Acquired: %d\n",
				stats.TotalConns(), stats.IdleConns(), stats.AcquiredConns())
		}
	}
	
	// Crear cliente Bedrock
	client := pkg.NewBedrockClient(config)
	
	// Inicializar middleware de autenticación (si BD disponible)
	var authMiddleware *auth.AuthMiddleware
	
	if db != nil {
		fmt.Println("🔐 Inicializando middleware de autenticación...")
		
		// Cargar config JWT con validación de seguridad
		jwtConfig, err := pkg.LoadJWTConfigWithEnv()
		if err != nil {
			// Error crítico: JWT_SECRET_KEY no cumple requisitos de seguridad
			fmt.Printf("❌ Error crítico en configuración JWT: %v\n", err)
			fmt.Println("💡 Solución:")
			fmt.Println("   1. Asegúrate de que JWT_SECRET_KEY esté configurado en AWS Secrets Manager")
			fmt.Println("   2. El secret debe tener al menos 32 caracteres")
			fmt.Println("   3. En ECS Task Definition, configura:")
			fmt.Println("      \"secrets\": [{")
			fmt.Println("        \"name\": \"JWT_SECRET_KEY\",")
			fmt.Println("        \"valueFrom\": \"arn:aws:secretsmanager:REGION:ACCOUNT:secret:bedrock-proxy/jwt-secret\"")
			fmt.Println("      }]")
			os.Exit(1)
		}
		
		// Convertir a auth.JWTConfig
		authConfig := auth.JWTConfig{
			SecretKey: jwtConfig.SecretKey,
			Issuer:    jwtConfig.Issuer,
			Audience:  jwtConfig.Audience,
		}
		
		authMiddleware = auth.NewAuthMiddleware(db, authConfig)
		
		// Configurar el logger en el middleware de autenticación
		auth.Logger = pkg.Logger
		
		fmt.Println("✅ Middleware de autenticación inicializado correctamente")
		fmt.Printf("✅ JWT Secret Key validado (longitud: %d caracteres)\n", len(jwtConfig.SecretKey))
		fmt.Println("✅ Verificación de cuota integrada en middleware de autenticación")
	}
	
	// Inicializar MetricsWorker y Scheduler (si BD disponible)
	var metricsWorker *metrics.MetricsWorker
	var schedulerService *scheduler.SchedulerService
	
	if db != nil {
		fmt.Println("📊 Inicializando MetricsWorker...")
		
		metricsConfig := metrics.DefaultConfig()
		metricsWorker = metrics.NewMetricsWorker(db, metricsConfig)
		metricsWorker.Start()
		
		fmt.Println("✅ MetricsWorker iniciado")
		
		// Inicializar Scheduler para reset diario
		fmt.Println("⏰ Inicializando Scheduler...")
		schedulerService = scheduler.NewSchedulerService(db, pkg.Log)
		schedulerService.Start()
		
		fmt.Println("✅ Scheduler iniciado")
	}
	
	// Pasar dependencias al cliente Bedrock y AuthMiddleware
	if db != nil && metricsWorker != nil {
		client.SetDependencies(db, metricsWorker)
		
		// Configurar MetricsWorker en AuthMiddleware para registro de errores tempranos
		if authMiddleware != nil {
			authMiddleware.SetMetricsWorker(metricsWorker)
			fmt.Println("✅ MetricsWorker configurado en AuthMiddleware para registro de errores")
		}
	}
	
	// Configurar rutas
	// Aplicar middleware de autenticación a /v1/messages si está disponible
	// NOTA: La verificación de cuota está integrada en el middleware de autenticación
	if authMiddleware != nil {
		fmt.Println("🔒 Aplicando autenticación (con verificación de cuota) a /v1/messages")
		
		middlewares := []func(http.Handler) http.Handler{
			authMiddleware.Middleware,
		}
		
		http.HandleFunc("/v1/messages", chainMiddlewares(client.HandleProxy, middlewares...))
	} else {
		fmt.Println("ℹ️  /v1/messages sin autenticación (modo legacy)")
		http.HandleFunc("/v1/messages", client.HandleProxy)
	}
	
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	fmt.Printf("🚀 Bedrock Proxy iniciado en puerto %s\n", port)
	fmt.Printf("📡 Endpoint: http://localhost:%s/v1/messages\n", port)
	fmt.Printf("🔧 Health check: http://localhost:%s/health\n", port)
	fmt.Printf("🌍 Región AWS: %s\n", config.Region)
	
	if config.DEBUG {
		fmt.Println("🐛 Modo DEBUG activado")
	}
	
	// Cerrar recursos al finalizar
	if db != nil {
		defer func() {
			if metricsWorker != nil {
				fmt.Println("📊 Deteniendo MetricsWorker...")
				metricsWorker.Stop()
			}
			if schedulerService != nil {
				fmt.Println("⏰ Deteniendo Scheduler...")
				schedulerService.Stop()
			}
			fmt.Println("🔌 Cerrando conexión a PostgreSQL...")
			db.Close()
			fmt.Println("✅ PostgreSQL desconectado")
		}()
	}
	
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
