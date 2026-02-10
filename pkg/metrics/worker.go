package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bedrock-proxy-test/pkg/database"
)

// MetricsWorker gestiona la inserción asíncrona de métricas
type MetricsWorker struct {
	db            *database.Database
	metricsChan   chan *database.MetricData
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
		metricsChan:   make(chan *database.MetricData, config.BufferSize),
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

	batch := make([]*database.MetricData, 0, mw.batchSize)
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
				batch = make([]*database.MetricData, 0, mw.batchSize)
			}

		case <-ticker.C:
			// Flush periódico aunque el batch no esté lleno
			if len(batch) > 0 {
				mw.flushBatch(batch)
				batch = make([]*database.MetricData, 0, mw.batchSize)
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
func (mw *MetricsWorker) flushBatch(batch []*database.MetricData) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insertar cada métrica (PostgreSQL maneja el particionamiento automáticamente)
	successCount := 0
	errorCount := 0

	for _, metric := range batch {
		if err := mw.db.InsertMetric(ctx, metric); err != nil {
			// Log error pero continuar con las demás métricas
			fmt.Printf("[MetricsWorker] Error inserting metric: %v\n", err)
			errorCount++
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		fmt.Printf("[MetricsWorker] Flushed batch: %d success, %d errors\n", successCount, errorCount)
	}
}

// RecordMetric añade una métrica al canal para procesamiento asíncrono
func (mw *MetricsWorker) RecordMetric(metric *database.MetricData) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	if mw.stopped {
		return fmt.Errorf("metrics worker is stopped")
	}

	// Intentar enviar al canal sin bloquear
	select {
	case mw.metricsChan <- metric:
		return nil
	default:
		// Canal lleno, métrica se pierde (o podríamos bloquear aquí)
		return fmt.Errorf("metrics channel is full, metric dropped")
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
