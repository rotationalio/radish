package radish

// This function returns the internal workers map for testing purposes but is not
// generally available as part of the public API.
func (r *Radish) Workers() *Workers {
	return r.workers
}
