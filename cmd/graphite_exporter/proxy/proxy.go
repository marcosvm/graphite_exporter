package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type Proxy struct {
	logger log.Logger
	c      chan string
	input  chan string
	mirror string
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

type CDF struct {
	Values         []float64 `json:"values"`
	DSTypes        []string  `json:"dstypes"`
	DSNames        []string  `json:"dsnames"`
	Time           float64   `json:"time"`
	Interval       float64   `json:"interval"`
	Host           string    `json:"host"`
	Plugin         string    `json:"plugin"`
	PluginInstance string    `json:"plugin_instance"`
	Type           string    `json:"type"`
	TypeInstance   string    `json:"type_instance"`
	Meta           struct{}  `json:"-"`
}

func (c *CDF) String() string {
	path := fmt.Sprintf("hosts.%s.%s", c.Host, c.Plugin)

	if c.PluginInstance != "" {
		path = fmt.Sprintf("%s-%s", path, c.PluginInstance)
	}

	path = fmt.Sprintf("%s.%s", path, c.Type)

	if c.TypeInstance != "" {
		typeInstance := strings.Replace(c.TypeInstance, ".", "_", -1)
		path = fmt.Sprintf("%s-%s", path, typeInstance)
	}

	i, d := math.Modf(c.Time)
	t := time.Unix(int64(i), int64(d*(1e9)))

	return fmt.Sprintf("%s %f %d", path, c.Values[0], t.Unix())
}

func NewProxy(logger log.Logger, c chan string, mirror string) *Proxy {
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
	return &Proxy{logger, c, input, mirror}
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
		case "SCDF":
			var m []CDF
			err := json.NewDecoder(bytes.NewReader(buf)).Decode(&m)
			if err != nil {
				level.Error(p.logger).Log("error decoding body", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, metric := range m {
				p.input <- metric.String()
			}
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
		if p.mirror != "" {
			go func(body []byte, mime string) {
				client := &http.Client{}
				req, err := http.NewRequest("POST", p.mirror, bytes.NewBuffer(buf))
				req.Header.Set("Content-Type", mime)
				resp, err := client.Do(req)
				if err != nil {
					level.Error(p.logger).Log("err", "error posting to mirror", "msg", err)
					return
				}
				defer func() {
					if resp != nil {
						resp.Body.Close()
					}
				}()

				_, err = io.ReadAll(resp.Body)
				if err != nil {
					level.Error(p.logger).Log("err", "error reading response", "msg", err)
					return
				}
			}(buf, mime)
		}
	}
}
