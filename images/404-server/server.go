/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// A webserver that only serves a 404 page. Used as a default backend.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var port = flag.Int("port", 8080, "Port number to serve default backend 404 page.")

func init() {
	// Register the summary and the histogram with Prometheus's default registry.
	prometheus.MustRegister(requestCount)
	prometheus.MustRegister(requestDuration)
}

func main() {
	flag.Parse()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// these headers and JSON response makes us look very similar to
		// a normal ESI 404 error, without tieing into the error limit system
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type,Authorization,X-User-Agent",
		)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set(
			"Access-Control-Expose-Headers",
			"Content-Type,Warning,X-Pages,"+
				"X-ESI-Error-Limit-Remain,X-ESI-Error-Limit-Reset",
		)
		w.Header().Set("Access-Control-Max-Age", "600")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")

		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "{\"error\": \"Not found\"}")

		proto := strconv.Itoa(r.ProtoMajor)
		proto = proto + "." + strconv.Itoa(r.ProtoMinor)

		requestCount.WithLabelValues(proto).Inc()

		duration := time.Now().Sub(start).Seconds() * 1e3
		requestDuration.WithLabelValues(proto).Observe(duration)
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	http.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", *port),
	}

	go func() {
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "could not start http server: %s\n", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := srv.Shutdown(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not graceful shutdown http server: %s\n", err)
	}
}
