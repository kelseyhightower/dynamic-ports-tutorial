package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"
)

const externalIPEndpoint = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip"
const internalIPEndpoint = "http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip"

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

	advertisedAddr := net.JoinHostPort(instanceIP, port)

	log.Printf("Registering advertised endpoint [%s]", advertisedAddr)
	_, err = http.Get(fmt.Sprintf("http://%s/register?name=%s&address=%s", serviceRegistryAddr, serviceInstanceName, advertisedAddr))
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

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
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
