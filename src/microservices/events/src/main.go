package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// Env helpers
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

type EventEnvelope struct {
	Type    string          `json:"type"`
	Key     string          `json:"key,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Ts      time.Time       `json:"ts"`
}

type LastStore struct {
	mu   sync.RWMutex
	data map[string]EventEnvelope
}
func NewLastStore() *LastStore { return &LastStore{data: make(map[string]EventEnvelope)} }
func (s *LastStore) Set(t string, ev EventEnvelope) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.data[strings.ToLower(t)] = ev
}
func (s *LastStore) Get(t string) (EventEnvelope, bool) {
	s.mu.RLock(); defer s.mu.RUnlock()
	ev, ok := s.data[strings.ToLower(t)]
	return ev, ok
}

// App config
type Config struct {
	Port           int
	Brokers        []string
	GroupID        string
	TopicUser      string
	TopicPayment   string
	TopicMovie     string
	ProduceTimeout time.Duration
	ConsumeStart   string // "latest" or "first"
}

func loadConfig() Config {
	brokersCSV := getenv("KAFKA_BROKERS", "kafka:9092")
	var brokers []string
	for _, b := range strings.Split(brokersCSV, ",") {
		b = strings.TrimSpace(b)
		if b != "" {
			brokers = append(brokers, b)
		}
	}
	startOffset := strings.ToLower(getenv("KAFKA_CONSUME_START", "latest"))
	return Config{
		Port:           getenvInt("PORT", 8082),
		Brokers:        brokers,
		GroupID:        getenv("KAFKA_GROUP_ID", "events-service-group"),
		TopicUser:      getenv("TOPIC_USER", "events.user"),
		TopicPayment:   getenv("TOPIC_PAYMENT", "events.payment"),
		TopicMovie:     getenv("TOPIC_MOVIE", "events.movie"),
		ProduceTimeout: time.Duration(getenvInt("PRODUCE_TIMEOUT_MS", 5000)) * time.Millisecond,
		ConsumeStart:   startOffset,
	}
}

// Producer
type Producer struct {
	cfg Config
	w   *kafka.Writer
}
func NewProducer(cfg Config) *Producer {
	return &Producer{
		cfg: cfg,
		w: &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			RequiredAcks: kafka.RequireOne,
			BatchTimeout: 10 * time.Millisecond,
			Async:        false,
		},
	}
}
func (p *Producer) Close() error { return p.w.Close() }
func (p *Producer) Produce(ctx context.Context, topic, key string, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.ProduceTimeout); defer cancel()
	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
		Time:  time.Now(),
	}
	return p.w.WriteMessages(ctx, msg)
}

// Consumers (single group for 3 topics)
func startConsumer(ctx context.Context, cfg Config, store *LastStore) {
	topics := []string{cfg.TopicUser, cfg.TopicPayment, cfg.TopicMovie}
	var start int64
	if cfg.ConsumeStart == "first" {
		start = kafka.FirstOffset
	} else {
		start = kafka.LastOffset
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.Brokers,
		GroupID:     cfg.GroupID,
		GroupTopics: topics,
		StartOffset: start,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	log.Printf("[consumer] start group=%s topics=%v startOffset=%v", cfg.GroupID, topics, cfg.ConsumeStart)
	go func() {
		defer r.Close()
		for {
			m, err := r.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					log.Printf("[consumer] context ended: %v", ctx.Err())
					return
				}
				log.Printf("[consumer] read error: %v", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}
			etype := ""
			switch m.Topic {
			case cfg.TopicUser:
				etype = "user"
			case cfg.TopicPayment:
				etype = "payment"
			case cfg.TopicMovie:
				etype = "movie"
			default:
				etype = "unknown"
			}

			env := EventEnvelope{
				Type:    etype,
				Key:     string(m.Key),
				Payload: json.RawMessage(m.Value),
				Ts:      time.Now(),
			}
			store.Set(etype, env)
			log.Printf("[consumer] topic=%s key=%s value=%s", m.Topic, string(m.Key), string(m.Value))
		}
	}()
}

// HTTP helpers
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	cfg := loadConfig()
	log.Printf("[events] starting on :%d, brokers=%v", cfg.Port, cfg.Brokers)

	store := NewLastStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prod := NewProducer(cfg)
	defer prod.Close()

	startConsumer(ctx, cfg, store)

	// Routes
	mux := http.NewServeMux()

	// Health endpoints to satisfy tests
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": true})
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": true})
	})
	mux.HandleFunc("/api/events/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": true})
	})

	// Diagnostics
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("/__config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"brokers": cfg.Brokers,
			"topics": map[string]string{
				"user": cfg.TopicUser, "payment": cfg.TopicPayment, "movie": cfg.TopicMovie,
			},
			"groupId": cfg.GroupID,
		})
	})

	// POST /api/events/{type}
	mux.HandleFunc("/api/events/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }
		publishEvent201(w, r, prod, cfg.TopicUser, "user")
	})
	mux.HandleFunc("/api/events/payment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }
		publishEvent201(w, r, prod, cfg.TopicPayment, "payment")
	})
	mux.HandleFunc("/api/events/movie", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { http.Error(w, "method not allowed", 405); return }
		publishEvent201(w, r, prod, cfg.TopicMovie, "movie")
	})

	// GET /api/events/last/{type}
	mux.HandleFunc("/api/events/last/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/events/last/"), "/")
		if len(parts) == 0 || parts[0] == "" { http.Error(w, "type required", 400); return }
		typ := strings.ToLower(parts[0])
		if ev, ok := store.Get(typ); ok {
			writeJSON(w, 200, ev); return
		}
		http.Error(w, "not found", 404)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// publishEvent201: returns 201 + {"status":"success", ...}
func publishEvent201(w http.ResponseWriter, r *http.Request, prod *Producer, topic string, etype string) {
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && err.Error() != "EOF" {
		http.Error(w, "bad json", 400); return
	}
	if in == nil { in = map[string]any{} }
	if _, ok := in["type"]; !ok { in["type"] = etype }
	if _, ok := in["ts"]; !ok { in["ts"] = time.Now().UTC().Format(time.RFC3339Nano) }

	key := ""
	if v, ok := in["key"].(string); ok { key = v }

	payload, _ := json.Marshal(in)
	if err := prod.Produce(r.Context(), topic, key, payload); err != nil {
		log.Printf("[producer] error: %v", err)
		http.Error(w, "kafka write failed", 502); return
	}
	log.Printf("[producer] topic=%s key=%s value=%s", topic, key, string(payload))
	resp := map[string]any{
		"status":  "success",
		"topic":   topic,
		"key":     key,
		"payload": in,
	}
	writeJSON(w, 201, resp)
}
