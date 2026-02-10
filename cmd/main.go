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
	"bedrock-proxy-test/pkg/quota"
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
	dbConfig := pkg.LoadDatabaseConfigWithEnv()
	
	if dbConfig.Host != "" && dbConfig.User != "" && dbConfig.Password != "" {
		var err error
		db, err = database.NewDatabase(dbConfig)
		if err != nil {
			fmt.Printf("⚠️  Error conectando a PostgreSQL: %v\n", err)
			fmt.Println("⚠️  El proxy continuará sin funcionalidades de BD (sin auth/cuotas)")
			db = nil
		} else {
			// Verificar conectividad
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
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
	} else {
		fmt.Println("ℹ️  Variables de BD no configuradas, continuando sin BD")
	}
	
	// Crear cliente Bedrock
	client := pkg.NewBedrockClient(config)
	
	// Inicializar middlewares (si BD disponible)
	var authMiddleware *auth.AuthMiddleware
	var quotaMiddleware *quota.QuotaMiddleware
	
	if db != nil {
		fmt.Println("🔐 Inicializando middlewares de autenticación...")
		
		// Cargar config JWT
		jwtConfig := pkg.LoadJWTConfigWithEnv()
		
		if jwtConfig.SecretKey != "" {
			// Convertir a auth.JWTConfig
			authConfig := auth.JWTConfig{
				SecretKey: jwtConfig.SecretKey,
				Issuer:    jwtConfig.Issuer,
				Audience:  jwtConfig.Audience,
			}
			
			authMiddleware = auth.NewAuthMiddleware(db, authConfig)
			quotaMiddleware = quota.NewQuotaMiddleware(db)
			
			fmt.Println("✅ Middlewares inicializados")
		} else {
			fmt.Println("⚠️  JWT_SECRET_KEY no configurado, continuando sin auth")
		}
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
	
	// Pasar dependencias al cliente Bedrock
	if db != nil && metricsWorker != nil {
		client.SetDependencies(db, metricsWorker, quotaMiddleware)
	}
	
	// Configurar rutas
	// Aplicar middlewares a /v1/messages si están disponibles
	if authMiddleware != nil && quotaMiddleware != nil {
		fmt.Println("🔒 Aplicando autenticación y control de cuotas a /v1/messages")
		
		middlewares := []func(http.Handler) http.Handler{
			authMiddleware.Middleware,
			quotaMiddleware.Middleware,
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
