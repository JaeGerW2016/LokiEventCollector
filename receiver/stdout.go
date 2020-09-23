package receiver

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
)

type StdoutTarget struct {
}

func NewStdoutTarget() (*StdoutTarget, error) {
	return &StdoutTarget{}, nil
}

func (st *StdoutTarget) Name() string {
	return "stdout"
}

func (st *StdoutTarget) Send(e *v1.Event) error {
	toSend, err := json.Marshal(e)
	if err != nil {
		return err
	}
	logrus.Infof("event: %s", toSend)
	return nil

}

func (st *StdoutTarget) Filter(e *v1.Event) bool {
	return true
}

func (st *StdoutTarget) Close() {

}
