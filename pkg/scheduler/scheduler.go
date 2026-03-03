package scheduler

import (
	"bedrock-proxy-test/pkg/database"
	"context"
	"time"
)

// Logger interface para logging
type Logger interface {
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
}

// SchedulerService gestiona tareas programadas
type SchedulerService struct {
	db     *database.Database
	logger Logger
	stopCh chan struct{}
}

// ResetResult contiene los resultados del reset diario
type ResetResult struct {
	UsersReset     int
	UsersUnblocked int
	CountersReset  int
	ExecutionTime  time.Duration
}

// NewSchedulerService crea una nueva instancia del scheduler
func NewSchedulerService(db *database.Database, logger Logger) *SchedulerService {
	return &SchedulerService{
		db:     db,
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start inicia todos los schedulers
func (s *SchedulerService) Start() {
	s.logger.Info("Starting scheduler service...")
	
	// Scheduler para reset diario a medianoche UTC
	go s.runDailyResetScheduler()
	
	s.logger.Info("Scheduler service started successfully")
}

// Stop detiene todos los schedulers
func (s *SchedulerService) Stop() {
	s.logger.Info("Stopping scheduler service...")
	close(s.stopCh)
}

// runDailyResetScheduler ejecuta el reset diario a medianoche UTC
func (s *SchedulerService) runDailyResetScheduler() {
	for {
		// Calcular tiempo hasta medianoche UTC
		now := time.Now().UTC()
		nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		duration := nextMidnight.Sub(now)
		
		s.logger.Infof("Next daily reset scheduled in %v (at %v UTC)", duration, nextMidnight.Format("2006-01-02 15:04:05"))
		
		// Esperar hasta medianoche o hasta que se detenga el servicio
		select {
		case <-time.After(duration):
			// Ejecutar reset diario
			s.logger.Info("Executing daily reset...")
			if err := s.RunDailyReset(context.Background()); err != nil {
				s.logger.Errorf("Failed to execute daily reset: %v", err)
			} else {
				s.logger.Info("Daily reset completed successfully")
			}
		case <-s.stopCh:
			s.logger.Info("Daily reset scheduler stopped")
			return
		}
	}
}

// RunDailyReset ejecuta el reset de contadores diarios
// NOTA: Con el nuevo sistema, el reset diario se hace automáticamente
// mediante la función PostgreSQL check_and_update_quota() que detecta
// cambios de día y resetea los contadores. Este scheduler se mantiene
// para logging y monitoreo, pero no ejecuta acciones en BD.
func (s *SchedulerService) RunDailyReset(ctx context.Context) error {
	startTime := time.Now()
	s.logger.Info("Daily reset checkpoint - counters reset automatically by PostgreSQL function")
	
	// El reset real se hace automáticamente en check_and_update_quota()
	// cuando detecta que es un nuevo día (current_date > last_reset_date)
	
	// Aquí podríamos añadir métricas o logs adicionales si es necesario
	duration := time.Since(startTime)
	s.logger.Infof("Daily reset checkpoint completed in %v", duration)
	s.logger.Info("Note: Actual counter reset happens automatically in PostgreSQL")
	
	return nil
}
