package main

import (
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/matt-deboer/go-marathon"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	listenAddress = flag.String(
		"web.listen-address",
		defaultEnv("WEB_LISTEN_ADDRESS", ":9088"),
		"Address to listen on for web interface and telemetry. [env: WEB_LISTEN_ADDRESS]")

	metricsPath = flag.String(
		"web.telemetry-path",
		defaultEnv("WEB_TELEMETRY_PATH", "/metrics"),
		"Path under which to expose metrics. [env: WEB_TELEMETRY_PATH]")

	marathonUri = flag.String(
		"marathon.uri",
		defaultEnv("MARATHON_URI", "http://marathon.mesos:8080"),
		"URI of Marathon. [env: MARATHON_URI]")
)

func defaultEnv(envVar, defaultVal string) string {
	if val, ok := os.LookupEnv(envVar); ok {
		return val
	}
	return defaultVal
}

func marathonConnect(uri *url.URL) error {
	config := marathon.NewDefaultConfig()
	config.URL = uri.String()

	if uri.User != nil {
		if passwd, ok := uri.User.Password(); ok {
			config.HTTPBasicPassword = passwd
			config.HTTPBasicAuthUser = uri.User.Username()
		}
	}
	config.HTTPClient = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).Dial,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	log.Debugln("Connecting to Marathon")
	client, err := marathon.NewClient(config)
	if err != nil {
		return err
	}

	info, err := client.Info()
	if err != nil {
		return err
	}

	log.Debugf("Connected to Marathon! Name=%s, Version=%s\n", info.Name, info.Version)
	return nil
}

func main() {
	flag.Parse()
	uri, err := url.Parse(*marathonUri)
	if err != nil {
		log.Fatal(err)
	}

	retryTimeout := time.Duration(10 * time.Second)
	for {
		err := marathonConnect(uri)
		if err == nil {
			break
		}

		log.Debugf("Problem connecting to Marathon: %v", err)
		log.Infof("Couldn't connect to Marathon! Trying again in %v", retryTimeout)
		time.Sleep(retryTimeout)
	}

	exporter := NewExporter(&scraper{uri}, defaultNamespace)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Marathon Exporter</title></head>
             <body>
             <h1>Marathon Exporter</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	log.Info("Starting Server: ", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
