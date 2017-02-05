package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"reflect"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

const (
	metadataHost    = "http://metadata.google.internal"
	projectEndpoint = "/computeMetadata/v1/project/project-id"
	networkEndpoint = "/computeMetadata/v1/instance/network-interfaces/0/network"
)

var (
	apiAddr    string
	httpAddr   string
	httpClient *http.Client
	network    string
	project    string
)

var html = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <style>
      th, td {
        text-align: left;
        padding-right: 1.5em;
      }
    </style>
    <title>Registry</title>
  </head>
  <body>
    <h1>Service Registry</h1>
    <h2>Backends</h2>
	<table>
      <tr>
        <th>Name</th>
        <th>Address</th>
        <th>Status</th>
      </tr>
	{{range $name, $endpoint := . }}
      <tr>
        <td>{{$name}}</td>
        <td><a href="http://{{$endpoint.Address}}">{{$endpoint.Address}}</a></td>
        <td></td>
      </tr>
    {{end}}
    </table>
  <body>
</html>
`

type Endpoint struct {
	Name    string
	Address string
	Tags    []string
}

func main() {
	flag.StringVar(&apiAddr, "api", "127.0.0.1:10081", "API listen address")
	flag.StringVar(&httpAddr, "http", "127.0.0.1:10080", "HTTP listen address")
	flag.Parse()

	log.Println("Starting service registry...")

	var err error

	timeout := 5 * time.Second
	httpClient = &http.Client{
		Timeout: timeout,
	}

	project, err = getProject()
	if err != nil {
		log.Fatal(err)
	}

	bm := newBackendManager()
	go bm.healthChecks()

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var endpoint Endpoint
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&endpoint)
		if err != nil {
			log.Println(err)
			w.WriteHeader(500)
			return
		}

		bm.add(endpoint)
	})

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("registry").Parse(html)
		if err != nil {
			log.Println(err)
		}

		err = t.Execute(w, bm.getBackends())
		if err != nil {
			log.Println(err)
		}
	})

	apiServer := http.Server{
		Addr:    apiAddr,
		Handler: apiMux,
	}

	httpServer := http.Server{
		Addr:    httpAddr,
		Handler: httpMux,
	}

	go func(){
		log.Fatal(apiServer.ListenAndServe())
	}()

	log.Fatal(httpServer.ListenAndServe())
}

func getInstanceNetwork() (string, error) {
	return getVauleFromMetadata(networkEndpoint)
}

func getProject() (string, error) {
	return getVauleFromMetadata(projectEndpoint)
}

func getVauleFromMetadata(path string) (string, error) {
	u := fmt.Sprintf("%s/%s", metadataHost, path)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("error fetching %s: %d", path, resp.StatusCode)
	}

	value, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(value), nil
}

func deleteFirewallRule(name string) error {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return err
	}

	c, err := compute.New(hc)
	if err != nil {
		return err
	}

	_, err = c.Firewalls.Delete(project, name).Context(ctx).Do()
	if err != nil {
		return err
	}

	return nil
}

func createFirewallRule(endpoint Endpoint) error {
	log.Printf("creating firewall rule for %s [%s]", endpoint.Name, endpoint.Address)
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, compute.CloudPlatformScope)
	if err != nil {
		return err
	}

	c, err := compute.New(hc)
	if err != nil {
		return err
	}

	_, port, err := net.SplitHostPort(endpoint.Address)
	if err != nil {
		return err
	}

	fwr := &compute.Firewall{
		Name:        endpoint.Name,
		Description: fmt.Sprintf("Allow %s from anywhere", port),
		Allowed: []*compute.FirewallAllowed{
			&compute.FirewallAllowed{
				IPProtocol: "tcp",
				Ports:      []string{port},
			},
		},
		SourceRanges: []string{"0.0.0.0/0"},
		TargetTags:   endpoint.Tags,
	}

	_, err = c.Firewalls.Insert(project, fwr).Context(ctx).Do()
	if err != nil {
		return err
	}

	return nil
}

type BackendManager struct {
	backends map[string]Endpoint
	m        *sync.Mutex
}

func newBackendManager() *BackendManager {
	return &BackendManager{
		backends: make(map[string]Endpoint),
		m:        &sync.Mutex{},
	}
}

func (bm *BackendManager) add(endpoint Endpoint) {
	bm.m.Lock()

	ep, ok := bm.backends[endpoint.Name]
	if ok {
		if reflect.DeepEqual(endpoint, ep) {
			bm.m.Unlock()
			return
		}
	}

	bm.backends[endpoint.Name] = endpoint
	bm.m.Unlock()

	err := createFirewallRule(endpoint)
	if err != nil {
		log.Println(err)
	}
}

func (bm *BackendManager) delete(name string) {
	bm.m.Lock()
	delete(bm.backends, name)
	bm.m.Unlock()

	err := deleteFirewallRule(name)
	if err != nil {
		log.Println(err)
	}
}

func (bm *BackendManager) getBackends() map[string]Endpoint {
	m := make(map[string]Endpoint)
	bm.m.Lock()
	for k, v := range bm.backends {
		m[k] = v
	}
	bm.m.Unlock()
	return m
}

func (bm *BackendManager) healthChecks() {
	for {
		for name, endpoint := range bm.backends {
			var healthy bool
			for i := 0; i <= 3; i++ {
				resp, err := httpClient.Get(fmt.Sprintf("http://%s/healthz", endpoint.Address))
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
