package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"bedrock-proxy-test/pkg"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var err error
	db, err = pkg.InitializeDatabase(ctx)
	if err != nil {
		db = nil
	} else if err := db.Ping(ctx); err != nil {
		db.Close()
		db = nil
	}
	
	// Crear cliente Bedrock
	client := pkg.NewBedrockClient(config)
	
	// Inicializar middleware de autenticación (si BD disponible)
	var authMiddleware *auth.AuthMiddleware
	
	if db != nil {
		jwtConfig, err := pkg.LoadJWTConfigWithEnv()
		if err != nil {
			fmt.Printf("Error: JWT configuration failed: %v\n", err)
			os.Exit(1)
		}
		
		authConfig := auth.JWTConfig{
			SecretKey: jwtConfig.SecretKey,
			Issuer:    jwtConfig.Issuer,
			Audience:  jwtConfig.Audience,
		}
		
		authMiddleware = auth.NewAuthMiddleware(db, authConfig)
		auth.Logger = pkg.Logger
	}
	
	// Inicializar MetricsWorker y Scheduler (si BD disponible)
	var metricsWorker *metrics.MetricsWorker
	var schedulerService *scheduler.SchedulerService
	
	if db != nil {
		metricsConfig := metrics.DefaultConfig()
		metricsWorker = metrics.NewMetricsWorker(db, metricsConfig)
		metricsWorker.Start()
		
		schedulerService = scheduler.NewSchedulerService(db, pkg.Log)
		schedulerService.Start()
	}
	
	// Pasar dependencias al cliente Bedrock y AuthMiddleware
	if db != nil && metricsWorker != nil {
		client.SetDependencies(db, metricsWorker)
		if authMiddleware != nil {
			authMiddleware.SetMetricsWorker(metricsWorker)
		}
	}
	
	// Configurar rutas
	if authMiddleware != nil {
		middlewares := []func(http.Handler) http.Handler{
			authMiddleware.Middleware,
		}
		http.HandleFunc("/v1/messages", chainMiddlewares(client.HandleProxy, middlewares...))
	} else {
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
	
	// Cerrar recursos al finalizar
	if db != nil {
		defer func() {
			if metricsWorker != nil {
				metricsWorker.Stop()
			}
			if schedulerService != nil {
				schedulerService.Stop()
			}
			db.Close()
		}()
	}
	
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
