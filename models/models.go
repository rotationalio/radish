package models

type Model interface {
	Scan(Scanner) error
}

// Scanner is an interface for *sql.Rows and *sql.Row so that models can implement how
// they scan fields into their struct without having to specify every field every time.
type Scanner interface {
	Scan(dest ...any) error
}
