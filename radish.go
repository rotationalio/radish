package radish

type Radish struct {
}

func New() (*Radish, error) {
	return &Radish{}, nil
}
