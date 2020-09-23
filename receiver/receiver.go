package receiver

import "k8s.io/api/core/v1"

type Receiver interface {
	Name() string
	Send(e *v1.Event) error
	Filter(e *v1.Event) bool
	Close()
}
