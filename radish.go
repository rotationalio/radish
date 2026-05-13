package radish

type Radish struct {
	workers *Workers
}

func New() (*Radish, error) {
	return &Radish{
		workers: &Workers{
			workers: make(map[string]untypedWorker),
		},
	}, nil
}

func (r *Radish) Workers() *Workers {
	return r.workers
}
