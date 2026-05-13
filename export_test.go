package radish

// This function returns the internal workers map for testing purposes but is not
// generally available as part of the public API.
func (r *Radish) Workers() *Workers {
	return r.workers
}

// Mark the Radish struct as running for testing purposes but is not
func (r *Radish) MarkRunning() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors = make([]*executor, 1)
}
