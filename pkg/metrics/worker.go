package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bedrock-proxy-test/pkg/database"
)

// MetricsWorker gestiona la inserción asíncrona de métricas de uso
type MetricsWorker struct {
	db            *database.Database
	metricsChan   chan *database.UsageTrackingData
	batchSize     int
	flushInterval time.Duration
	wg            sync.WaitGroup
	stopChan      chan struct{}
	stopped       bool
	mu            sync.Mutex
}

// Config contiene la configuración del worker de métricas
type Config struct {
	BufferSize    int           // Tamaño del canal buffered
	BatchSize     int           // Número de métricas por batch
	FlushInterval time.Duration // Intervalo de flush automático
}

// DefaultConfig retorna la configuración por defecto
func DefaultConfig() Config {
	return Config{
		BufferSize:    1000,         // Buffer para 1000 métricas
		BatchSize:     50,           // Insertar cada 50 métricas
		FlushInterval: 5 * time.Second, // O cada 5 segundos
	}
}

// NewMetricsWorker crea una nueva instancia del worker de métricas
func NewMetricsWorker(db *database.Database, config Config) *MetricsWorker {
	return &MetricsWorker{
		db:            db,
		metricsChan:   make(chan *database.UsageTrackingData, config.BufferSize),
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		stopChan:      make(chan struct{}),
		stopped:       false,
	}
}

// Start inicia el worker de métricas
func (mw *MetricsWorker) Start() {
	mw.wg.Add(1)
	go mw.run()
}

// run es el loop principal del worker
func (mw *MetricsWorker) run() {
	defer mw.wg.Done()

	batch := make([]*database.UsageTrackingData, 0, mw.batchSize)
	ticker := time.NewTicker(mw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case metric := <-mw.metricsChan:
			// Añadir métrica al batch
			batch = append(batch, metric)

			// Si el batch está lleno, insertar
			if len(batch) >= mw.batchSize {
				mw.flushBatch(batch)
				batch = make([]*database.UsageTrackingData, 0, mw.batchSize)
			}

		case <-ticker.C:
			// Flush periódico aunque el batch no esté lleno
			if len(batch) > 0 {
				mw.flushBatch(batch)
				batch = make([]*database.UsageTrackingData, 0, mw.batchSize)
			}

		case <-mw.stopChan:
			// Flush final antes de cerrar
			if len(batch) > 0 {
				mw.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch inserta un batch de métricas en la base de datos
func (mw *MetricsWorker) flushBatch(batch []*database.UsageTrackingData) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insertar cada métrica usando la nueva tabla de usage tracking
	successCount := 0
	errorCount := 0

	for _, metric := range batch {
		// Log detallado de cada métrica antes de insertar
		fmt.Printf("[MetricsWorker] Inserting usage tracking: user=%s, model=%s, tokens_in=%d, tokens_out=%d\n",
			metric.CognitoUserID, metric.ModelID, metric.TokensInput, metric.TokensOutput)
		
		if err := mw.db.InsertUsageTracking(ctx, metric); err != nil {
			// Log error detallado
			fmt.Printf("[MetricsWorker] ❌ Error inserting usage tracking: %v\n", err)
			fmt.Printf("[MetricsWorker] Failed metric details: user=%s, email=%s, model=%s\n",
				metric.CognitoUserID, metric.CognitoEmail, metric.ModelID)
			errorCount++
		} else {
			fmt.Printf("[MetricsWorker] ✅ Successfully inserted usage tracking for user %s\n", metric.CognitoUserID)
			successCount++
		}
	}

	if successCount > 0 || errorCount > 0 {
		fmt.Printf("[MetricsWorker] Batch flush complete: %d success, %d errors\n", successCount, errorCount)
	}
}

// RecordMetric añade una métrica al canal para procesamiento asíncrono
// Deprecated: Use RecordUsageTracking instead
func (mw *MetricsWorker) RecordMetric(metric *database.MetricData) error {
	// Convertir MetricData a UsageTrackingData para compatibilidad
	usageData := &database.UsageTrackingData{
		CognitoUserID:       metric.UserID,
		CognitoEmail:        "", // No disponible en MetricData antigua
		RequestTimestamp:    metric.RequestTimestamp,
		ModelID:             metric.ModelID,
		SourceIP:            metric.SourceIP,
		UserAgent:           metric.UserAgent,
		AWSRegion:           metric.AWSRegion,
		TokensInput:         metric.TokensInput,
		TokensOutput:        metric.TokensOutput,
		TokensCacheRead:     metric.TokensCacheRead,
		TokensCacheCreation: metric.TokensCacheCreation,
		CostUSD:             metric.CostUSD,
		ProcessingTimeMS:    metric.ProcessingTimeMS,
		ResponseStatus:      metric.ResponseStatus,
		ErrorMessage:        metric.ErrorMessage,
	}
	
	return mw.RecordUsageTracking(usageData)
}

// RecordUsageTracking añade datos de uso al canal para procesamiento asíncrono
func (mw *MetricsWorker) RecordUsageTracking(data *database.UsageTrackingData) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	if mw.stopped {
		return fmt.Errorf("metrics worker is stopped")
	}

	// Intentar enviar al canal sin bloquear
	select {
	case mw.metricsChan <- data:
		return nil
	default:
		// Canal lleno, métrica se pierde (o podríamos bloquear aquí)
		return fmt.Errorf("metrics channel is full, usage tracking dropped")
	}
}

// Stop detiene el worker de métricas de forma graceful
func (mw *MetricsWorker) Stop() {
	mw.mu.Lock()
	if mw.stopped {
		mw.mu.Unlock()
		return
	}
	mw.stopped = true
	mw.mu.Unlock()

	// Señalar al worker que debe detenerse
	close(mw.stopChan)

	// Esperar a que termine el flush final
	mw.wg.Wait()

	// Cerrar el canal de métricas
	close(mw.metricsChan)

	fmt.Println("[MetricsWorker] Stopped gracefully")
}

// Stats retorna estadísticas del worker
func (mw *MetricsWorker) Stats() WorkerStats {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	return WorkerStats{
		BufferSize:     cap(mw.metricsChan),
		BufferedCount:  len(mw.metricsChan),
		BatchSize:      mw.batchSize,
		FlushInterval:  mw.flushInterval,
		IsStopped:      mw.stopped,
	}
}

// WorkerStats contiene estadísticas del worker
type WorkerStats struct {
	BufferSize     int
	BufferedCount  int
	BatchSize      int
	FlushInterval  time.Duration
	IsStopped      bool
}
