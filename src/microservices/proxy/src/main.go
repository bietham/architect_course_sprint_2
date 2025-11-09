package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	mrand "math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type targets struct {
	Monolith *url.URL
	Movies   *url.URL
	Events   *url.URL
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func getenvInt(key string, def int) int {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}
func getenvBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func mustParse(u string) *url.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		log.Fatalf("invalid url %s: %v", u, err)
	}
	return parsed
}

func newRID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), mrand.Int63())
}

func buildTransport(timeoutMS int) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(timeoutMS) * time.Millisecond,
		}).DialContext,
		TLSHandshakeTimeout:   time.Duration(timeoutMS) * time.Millisecond,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeoutMS) * time.Millisecond,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
	}
}

// ---- logging response writer
type loggingRW struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingRW) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *loggingRW) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func ensureReqID(r *http.Request) string {
	id := r.Header.Get("X-Request-Id")
	if id == "" {
		id = newRID()
		r.Header.Set("X-Request-Id", id)
	}
	return id
}

// Универсальный логирующий вызов прокси с фиксированной целью (events/monolith)
func serveWithLog(targetName string, p *httputil.ReverseProxy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := ensureReqID(r)
		start := time.Now()
		lrw := &loggingRW{ResponseWriter: w}
		p.ServeHTTP(lrw, r)

		target := targetName
		if h := lrw.Header().Get("X-Proxy-Target"); h != "" {
			target = strings.ToLower(h)
		}
		log.Printf(`[access] req=%s ip=%s %s %s status=%d bytes=%d dur_ms=%d target=%s`,
			reqID, clientIP(r), r.Method, r.URL.RequestURI(),
			lrw.status, lrw.bytes, time.Since(start).Milliseconds(), target)
	}
}





func newProxy(target *url.URL, targetName string, transport *http.Transport) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		if req.Header.Get("X-Request-Id") == "" {
			req.Header.Set("X-Request-Id", newRID())
		}
		// Save original host before we override it
		origHost := req.Host

		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		if ip := clientIP(req); ip != "" {
			xff := req.Header.Get("X-Forwarded-For")
			if xff == "" {
				req.Header.Set("X-Forwarded-For", ip)
			} else {
				req.Header.Set("X-Forwarded-For", xff+", "+ip)
			}
		}
		if req.Header.Get("X-Forwarded-Proto") == "" {
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		if req.Header.Get("X-Forwarded-Host") == "" && origHost != "" {
			req.Header.Set("X-Forwarded-Host", origHost)
		}
	}

	proxy := &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
		ModifyResponse: func(r *http.Response) error {
			r.Header.Set("X-Proxy-Target", targetName)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[proxy %s] error: %v", targetName, err)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}
	return proxy
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

func main() {
	mrand.Seed(time.Now().UnixNano())

	port := getenvInt("PORT", 8000)
	monolithURL := getenv("MONOLITH_URL", "http://monolith:8080")
	moviesURL := getenv("MOVIES_SERVICE_URL", "http://movies-service:8081")
	eventsURL := getenv("EVENTS_SERVICE_URL", "http://events-service:8082")
	gradual := getenvBool("GRADUAL_MIGRATION", true)
	percent := getenvInt("MOVIES_MIGRATION_PERCENT", 50)
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	timeoutMS := getenvInt("FORWARD_TIMEOUT_MS", 8000)

	tgs := targets{
		Monolith: mustParse(monolithURL),
		Movies:   mustParse(moviesURL),
		Events:   mustParse(eventsURL),
	}
	transport := buildTransport(timeoutMS)
	monoProxy := newProxy(tgs.Monolith, "monolith", transport)
	moviesProxy := newProxy(tgs.Movies, "movies", transport)
	eventsProxy := newProxy(tgs.Events, "events", transport)

	// movies fallback -> monolith
	moviesProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Header.Get("X-Fallback-Attempted") == "" {
			log.Printf("[proxy movies] error: %v — falling back to monolith", err)
			r.Header.Set("X-Fallback-Attempted", "1")
			monoProxy.ServeHTTP(w, r)
			return
		}
		log.Printf("[proxy movies] fallback also failed: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/__routing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"gradualMigration":%v,"moviesMigrationPercent":%d,"targets":{"monolith":"%s","movies":"%s","events":"%s"}}`,
			gradual, percent, monolithURL, moviesURL, eventsURL)
	})

	// events direct
	mux.HandleFunc("/api/events",      serveWithLog("events",   eventsProxy))
	mux.HandleFunc("/api/events/",     serveWithLog("events",   eventsProxy))


	// movies with % split + override
	moviesHandler := func(w http.ResponseWriter, r *http.Request) {
		reqID := ensureReqID(r)
		start := time.Now()
		lrw := &loggingRW{ResponseWriter: w}

		// Header-override
		switch strings.ToLower(r.Header.Get("X-Force-Service")) {
		case "monolith":
			monoProxy.ServeHTTP(lrw, r)
		case "movies":
			moviesProxy.ServeHTTP(lrw, r)
		default:
			// процентная миграция
			if !gradual || mrand.Intn(100) >= percent {
				monoProxy.ServeHTTP(lrw, r)
			} else {
				moviesProxy.ServeHTTP(lrw, r)
			}
		}

		target := lrw.Header().Get("X-Proxy-Target") // учитывает фолбэк
		if target == "" {
			target = "unknown"
		}
		log.Printf(`[access] req=%s ip=%s %s %s status=%d bytes=%d dur_ms=%d target=%s`,
			reqID, clientIP(r), r.Method, r.URL.RequestURI(),
			lrw.status, lrw.bytes, time.Since(start).Milliseconds(), strings.ToLower(target))
	}
	mux.HandleFunc("/api/movies",  moviesHandler)
	mux.HandleFunc("/api/movies/", moviesHandler)

	// default -> monolith
	mux.HandleFunc("/",                serveWithLog("monolith", monoProxy))

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[proxy] Listening on %s", addr)
	log.Printf("[proxy] MONOLITH_URL=%s", monolithURL)
	log.Printf("[proxy] MOVIES_SERVICE_URL=%s", moviesURL)
	log.Printf("[proxy] EVENTS_SERVICE_URL=%s", eventsURL)
	log.Printf("[proxy] GRADUAL_MIGRATION=%v MOVIES_MIGRATION_PERCENT=%d", gradual, percent)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
