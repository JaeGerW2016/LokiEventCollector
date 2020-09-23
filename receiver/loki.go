package receiver

import (
	"LokiEventCollector/config"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	lokiclient "github.com/livepeer/loki-client/client"
	"github.com/livepeer/loki-client/model"
	"io"
	"k8s.io/api/core/v1"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const contentType = "application/json"
const maxErrMsgLen = 1024

type LokiTarget struct {
	client *lokiclient.Client
	config lokiclient.Config
}

func logger(v ...interface{}) {
	fmt.Println(v...)
}

func NewLokiTarget(conf *config.Loki) (*LokiTarget, error) {
	baseLabels := model.LabelSet{}
	lokiURL := conf.URL
	fmt.Printf("using Loki url: %s", lokiURL)
	client, err := lokiclient.NewWithDefaults(lokiURL, baseLabels, logger)
	if err != nil {
		return nil, err
	}
	lc := lokiclient.Config{
		URL: conf.URL,
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)

	go waitExit(client, c)

	return &LokiTarget{
		client: client,
		config: lc,
	}, nil

}

func waitExit(client *lokiclient.Client, c chan os.Signal) {
	<-c
	client.Stop()
}

func (lt *LokiTarget) Name() string {
	return "loki"
}

func (lt *LokiTarget) Send(e *v1.Event) error {
	toSend, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", lt.config.URL, bytes.NewBuffer(toSend))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = fmt.Errorf("server returned HTTP status %s (%d): %s", resp.Status, resp.StatusCode, line)
	}
	return err
}

func (lt *LokiTarget) Filter(e *v1.Event) bool {
	return true
}

func (lt *LokiTarget) Close() {

}
