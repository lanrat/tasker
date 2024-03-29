package jobs

// JobStats interface for stats to be collected externally
type JobStats interface {
	AddWorkers(int)
	AddError(int)
	AddJobQueueTotal(int)
	AddJobQueueSize(int)
	AddWork(int)
	AddWorkType(string)
	AddSavedInputs(int)
	AddCacheHits(int)
	AddResults(int)
}
