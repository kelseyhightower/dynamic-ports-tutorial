package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
)

var httpClient *http.Client

func main() {
	timeout := 5 * time.Second
	httpClient = &http.Client{
		Timeout: timeout,
	}

	bm := newBackendManager()
	go bm.healthChecks()

	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		bm.add(r.FormValue("backend"), r.FormValue("address"))
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

	log.Fatal(http.ListenAndServe("0.0.0.0:80", nil))
}

type BackendManager struct {
	backends map[string]string
	m        *sync.Mutex
}

func newBackendManager() *BackendManager {
	return &BackendManager{
		backends: make(map[string]string),
		m: &sync.Mutex{},
	}
}

func (bm *BackendManager) add(backend, address string) {
	bm.m.Lock()
	bm.backends[backend] = address
	bm.m.Unlock()
}

func (bm *BackendManager) delete(backend string) {
	bm.m.Lock()
	delete(bm.backends, backend)
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
		for backend, address := range bm.backends {
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
				bm.delete(backend)
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
	{{range $backend, $address := . }}
    <p>{{$backend}}: {{$address}}</p>
    {{end}}
  <body>
</html>
`
