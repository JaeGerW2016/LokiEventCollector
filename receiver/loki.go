package receiver

import (
	"LokiEventCollector/config"
	"bufio"
	"bytes"
	"context"
	"fmt"
	lokiclient "github.com/livepeer/loki-client/client"
	"github.com/livepeer/loki-client/model"
	"io"
	"k8s.io/api/core/v1"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
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
	parse := makeRequestBody(e)
	req, err := http.NewRequest("POST", lt.config.URL, bytes.NewBuffer(parse))
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

func formatEvent(e v1.Event) string {
	var b strings.Builder
	t := reflect.TypeOf(e)
	v := reflect.ValueOf(e)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).CanInterface() {
			if v.Field(i).Type().Kind() == reflect.Struct {
				structField := v.Field(i).Type()
				for j := 0; j < structField.NumField(); j++ {
					//filter string slice map struct which value is empty
					if interfaceValueAssert(v.Field(i).Field(j).Interface()) {
						continue
					}
					b.WriteString(fmt.Sprintf("%s:%v, ", structField.Field(j).Name, v.Field(i).Field(j).Interface()))
				}
				continue
			}
			if t.Field(i).Name == "Message" {
				m := trimQuotes(fmt.Sprintf("%v", v.Field(i).Interface()))
				b.WriteString(fmt.Sprintf("%s:%v, ", t.Field(i).Name, m))
				continue
			}
			if interfaceValueAssert(v.Field(i).Interface()) {
				continue
			}
			b.WriteString(fmt.Sprintf("%s:%v, ", t.Field(i).Name, v.Field(i).Interface()))
		}

	}
	return b.String()
}

func makeRequestBody(e *v1.Event) []byte {
	//Constructing Tags with Specific Tags: _kind, _namespace, _host, _type
	tags := `"_kind":"` + e.InvolvedObject.Kind + `",` + `"_namespace":"` + e.ObjectMeta.Namespace + `",` + `"_host":"` + e.Source.Host + `",` + `"_type":"` + e.Type + "\""
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	message := formatEvent(*e)
	param := []byte(`
	{
		"streams":[
			{
				"stream":{
					` + tags + `
				},
				"values":[
					["` + timestamp + `","` + message + `"]
				]
			}
		]
	}`)

	return param
}

// Remove double quotes from a string
func trimQuotes(s string) string {
	var b bytes.Buffer
	slice := strings.Fields(s)
	for _, v := range slice {
		v = strings.Trim(v, "\"")
		b.WriteString(" ")
		b.WriteString(v)
	}
	return b.String()
}

//Determine if interfaces are empty strings, empty slices, empty dictionaries, and illegal constructs.
func interfaceValueAssert(u interface{}) bool {
	switch reflect.TypeOf(u).Kind() {
	case reflect.String:
		if u == "" {
			return true
		}
	case reflect.Slice:
		s := reflect.ValueOf(u)
		if s.Len() == 0 {
			return true
		}
	case reflect.Map:
		m := reflect.ValueOf(u)
		if m.Len() == 0 {
			return true
		}
	case reflect.Struct:
		st := reflect.ValueOf(&u).Elem()
		if !st.IsValid() {
			return true
		}
	default:
		return false
	}
	return false
}

func (lt *LokiTarget) Filter(e *v1.Event) bool {
	return true
}

func (lt *LokiTarget) Close() {

}
