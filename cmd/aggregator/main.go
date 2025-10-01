package main

import (
	"bufio"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
)

const (
	serviceName = "aggregator"
	pprofAddr   = ":6061"
	envFilePath = "deploy/env/.env.example"
)

func main() {
	logger := log.New(os.Stdout, "["+serviceName+"] ", log.LstdFlags|log.Lmicroseconds)
	if err := loadEnvFile(logger, envFilePath); err != nil {
		logger.Printf("env load warning: %v", err)
	}
	logger.Printf("starting service: %s", serviceName)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/debug/pprof/", func(w http.ResponseWriter, r *http.Request) { http.DefaultServeMux.ServeHTTP(w, r) })

	go func() {
		logger.Printf("pprof/health HTTP listening on %s", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, mux); err != nil {
			logger.Printf("pprof server error: %v", err)
		}
	}()

	select {}
}

func loadEnvFile(logger *log.Logger, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := kv[1]
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
			logger.Printf("env set %s (from example)", key)
		}
	}
	return scanner.Err()
}
