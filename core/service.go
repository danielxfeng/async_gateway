package core

type Service interface {
	Name() string
	Run() error
	// Clean is called when the service exits abnormally.
	Clean()
}
