package receiver

import "k8s.io/api/core/v1"

type DiscardTarget struct {
}

func NewDiscardTarget() (*DiscardTarget, error) {
	return &DiscardTarget{}, nil
}

func (dt *DiscardTarget) Name() string {
	return "discard"
}

func (dt *DiscardTarget) Send(e *v1.Event) error {
	return nil
}

func (dt *DiscardTarget) Filter(e *v1.Event) bool {
	return true
}

func (dt *DiscardTarget) Close() {

}
