package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	metadataHost       = "http://metadata.google.internal"
	externalIPEndpoint = "/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip"
	internalIPEndpoint = "/computeMetadata/v1/instance/network-interfaces/0/ip"
	tagsEndpoint       = "/computeMetadata/v1/instance/tags"
)

var (
	advertisedIP        string
	instanceIP          string
	serviceRegistryAddr string
	registerExternalIP  bool
	serviceInstanceName string
)

var html = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>Dynamic Port Server</title>
  </head>
  <body>
    <h1>%s</h1>
    <p>%s</p>
  <body>
</html>
`

type Endpoint struct {
	Name    string
	Address string
	Tags    []string
}

func main() {
	flag.StringVar(&advertisedIP, "advertised-ip", "", "The advertised IP address to register.")
	flag.StringVar(&serviceRegistryAddr, "service-registry", "127.0.0.1:8888", "The remote service registry address.")
	flag.BoolVar(&registerExternalIP, "register-instance-external-ip", false, "Register the external IP address of the compute instance.")
	flag.StringVar(&serviceInstanceName, "service-instance-name", "", "The unique service instance name.")
	flag.Parse()

	log.Println("Starting dynamic port server...")

	var err error

	ln, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		log.Fatal(err)
	}

	hostPort := ln.Addr().String()
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Listening on %s", hostPort)

	if advertisedIP != "" {
		instanceIP = advertisedIP
	}

	if registerExternalIP {
		instanceIP, err = getInstanceExternalIP()
		if err != nil {
			log.Fatal(err)
		}
	}

	if instanceIP == "" {
		instanceIP, err = getInstanceIP()
		if err != nil {
			log.Fatal(err)
		}
	}

	tags, err := getInstanceTags()
	if err != nil {
		log.Fatal(err)
	}

	advertisedAddr := net.JoinHostPort(instanceIP, port)

	log.Printf("Registering endpoint [%s]", advertisedAddr)

	e := &Endpoint{
		Name:    serviceInstanceName,
		Address: advertisedAddr,
		Tags:    tags,
	}

	err = registerEndpoint(e)
	if err != nil {
		log.Fatal(err)
	}

	// Register HTTP Handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, html, serviceInstanceName, advertisedAddr)
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		format := "%s - - [%s] \"%s %s %s\" %s\n"
		fmt.Printf(format, r.RemoteAddr, time.Now().Format(time.RFC1123),
			r.Method, r.URL.Path, r.Proto, r.UserAgent())
	})

	s := &http.Server{}
	log.Fatal(s.Serve(ln))
}

func registerEndpoint(endpoint *Endpoint) error {
	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)

	err := enc.Encode(endpoint)
	if err != nil {
		return err
	}

	u := fmt.Sprintf("http://%s/register", serviceRegistryAddr)

	req, err := http.NewRequest(http.MethodPost, u, body)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("error registering endpoint: %d", resp.StatusCode)
	}

	return nil
}

func getInstanceExternalIP() (string, error) {
	return getInstanceIPFromMetadata(true)
}

func getInstanceIP() (string, error) {
	return getInstanceIPFromMetadata(false)
}

func getInstanceIPFromMetadata(external bool) (string, error) {
	endpoint := internalIPEndpoint
	if external {
		endpoint = externalIPEndpoint
	}

	u := fmt.Sprintf("%s/%s", metadataHost, endpoint)

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
		return "", fmt.Errorf("error retrieving instance IP: %d", resp.StatusCode)
	}

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(ip), nil
}

func getInstanceTags() ([]string, error) {
	return getInstanceTagsFromMetadata()
}

func getInstanceTagsFromMetadata() ([]string, error) {
	var tags []string

	u := fmt.Sprintf("%s/%s", metadataHost, tagsEndpoint)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return tags, err
	}
	req.Header.Add("Metadata-Flavor", "Google")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return tags, err
	}

	if resp.StatusCode != 200 {
		return tags, fmt.Errorf("error retrieving instance tags: %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&tags)
	if err != nil {
		return tags, err
	}

	return tags, nil
}
