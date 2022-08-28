package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type Proxy struct {
	logger log.Logger
	c      chan string
	input  chan string
}

type SFIA struct {
	Path      string  `json:"path"`
	Value     float64 `json:"value"`
	Timestamp int     `json:"timestamp"`
}

func (s *SFIA) String() string {
	return fmt.Sprintf("%s %f %d", s.Path, s.Value, s.Timestamp)
}

type SSSS struct {
	Path      string `json:"path"`
	Value     string `json:"value"`
	Timestamp string `json:"timestamp"`
}

func (s *SSSS) String() string {
	return fmt.Sprintf("%s %s %s", s.Path, s.Value, s.Timestamp)
}

func NewProxy(logger log.Logger, c chan string) *Proxy {
	var (
		bSize int
		err   error
	)

	size := os.Getenv("METRIC_BUFFER_SIZE")
	if bSize, err = strconv.Atoi(size); err != nil {
		bSize = 100
	}

	input := make(chan string, bSize)
	go func(chan string) {
		for {
			select {
			case m := <-input:
				c <- m
			}
		}

	}(input)
	return &Proxy{logger, c, input}
}

func (p *Proxy) Forward(w http.ResponseWriter, r *http.Request) {
	var (
		buf []byte
		err error
	)
	if r.Method == "POST" && r.URL.Path == "/metric" {
		buf, err = io.ReadAll(r.Body)
		if err != nil {
			level.Error(p.logger).Log("error reading body", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch ser := os.Getenv("SERIALIZATION"); ser {
		case "SFIA":
			var m []SFIA
			err := json.NewDecoder(bytes.NewReader(buf)).Decode(&m)
			if err != nil {
				level.Error(p.logger).Log("error decoding body", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, metric := range m {
				p.input <- metric.String()
			}
		case "SSSS":
			var m SSSS
			err := json.NewDecoder(bytes.NewReader(buf)).Decode(&m)
			if err != nil {
				level.Error(p.logger).Log("error decoding body", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			p.input <- m.String()
		default:
			var m []SSSS
			err := json.NewDecoder(bytes.NewReader(buf)).Decode(&m)
			if err != nil {
				level.Error(p.logger).Log("error decoding body", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, metric := range m {
				p.input <- metric.String()
			}
		}

		// if configured to mirror, send the request to its destination
		mime := r.Header.Get("Content-Type")
		mirror := os.Getenv("METRICS_MIRROR_URL")
		if mirror != "" {
			go func(body []byte, mime string) {
				client := &http.Client{}
				req, err := http.NewRequest("POST", mirror, bytes.NewBuffer(buf))
				req.Header.Set("Content-Type", mime)
				resp, err := client.Do(req)
				if err != nil {
					level.Error(p.logger).Log("err", "error posting to mirror", err)
					return
				}
				defer func() {
					if resp != nil {
						resp.Body.Close()
					}
				}()

				_, err = io.ReadAll(resp.Body)
				if err != nil {
					level.Error(p.logger).Log("err", "error reading response", err)
					return
				}
			}(buf, mime)
		}
	}
}
