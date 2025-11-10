package services

var axentaSyncScheduler *AxentaSyncScheduler

// SetAxentaSyncScheduler сохраняет глобальный планировщик синхронизации Axenta
func SetAxentaSyncScheduler(scheduler *AxentaSyncScheduler) {
	axentaSyncScheduler = scheduler
}

// GetAxentaSyncScheduler возвращает текущий планировщик синхронизации Axenta
func GetAxentaSyncScheduler() *AxentaSyncScheduler {
	return axentaSyncScheduler
}
