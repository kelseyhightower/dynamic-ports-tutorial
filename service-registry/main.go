package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	httpClient *http.Client
	listenAddr string
)

func main() {
	flag.StringVar(&listenAddr, "listen-addr", "127.0.0.1:8888", "HTTP listen address")
	flag.Parse()

	log.Println("Starting service registry...")

	timeout := 5 * time.Second
	httpClient = &http.Client{
		Timeout: timeout,
	}

	bm := newBackendManager()
	go bm.healthChecks()

	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		bm.add(r.FormValue("name"), r.FormValue("address"))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("registry").Parse(html)
		if err != nil {
			log.Println(err)
		}

		err = t.Execute(w, bm.getBackends())
		if err != nil {
			log.Println(err)
		}
	})

	log.Fatal(http.ListenAndServe(listenAddr, nil))
}

type BackendManager struct {
	backends map[string]string
	m        *sync.Mutex
}

func newBackendManager() *BackendManager {
	return &BackendManager{
		backends: make(map[string]string),
		m:        &sync.Mutex{},
	}
}

func (bm *BackendManager) add(name, address string) {
	bm.m.Lock()
	bm.backends[name] = address
	bm.m.Unlock()
}

func (bm *BackendManager) delete(name string) {
	bm.m.Lock()
	delete(bm.backends, name)
	bm.m.Unlock()
}

func (bm *BackendManager) getBackends() map[string]string {
	m := make(map[string]string)
	bm.m.Lock()
	for k, v := range bm.backends {
		m[k] = v
	}
	bm.m.Unlock()
	return m
}

func (bm *BackendManager) healthChecks() {
	for {
		for name, address := range bm.backends {
			var healthy bool
			for i := 0; i <= 3; i++ {
				resp, err := httpClient.Get(fmt.Sprintf("http://%s/healthz", address))
				if err != nil {
					log.Println(err)
					time.Sleep(3 * time.Second)
					continue
				}

				if resp.StatusCode != 200 {
					log.Println("health check failed for %s: non 200 HTTP response")
					time.Sleep(3 * time.Second)
					continue
				}

				healthy = true
				break
			}

			if !healthy {
				bm.delete(name)
			}
		}

		time.Sleep(10 * time.Second)
	}
}

var html = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>Registry</title>
  </head>
  <body>
    <h1>Service Registry</h1>
	{{range $name, $address := . }}
    <h2>Backends</h2>
    <p>{{$name}}: {{$address}}</p>
    {{end}}
  <body>
</html>
`
