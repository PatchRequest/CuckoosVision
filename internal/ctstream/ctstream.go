package ctstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"CuckoosVision/internal/domain"

	"github.com/coder/websocket"
)

type Handler func(ctx context.Context, d string)

type Config struct {
	URL string
}

func DefaultConfig() Config {
	return Config{
		URL: "wss://certstream.calidog.io",
	}
}

type certStreamMessage struct {
	MessageType string `json:"message_type"`
	Data        struct {
		LeafCert struct {
			AllDomains []string `json:"all_domains"`
		} `json:"leaf_cert"`
	} `json:"data"`
}

func Stream(ctx context.Context, cfg Config, handler Handler) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := connect(ctx, cfg.URL, handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		log.Printf("CertStream disconnected: %v, reconnecting in 5s...", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

func connect(ctx context.Context, url string, handler Handler) error {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()

	conn.SetReadLimit(1 << 20)

	seen := make(map[string]struct{})

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var msg certStreamMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.MessageType != "certificate_update" {
			continue
		}

		for _, d := range msg.Data.LeafCert.AllDomains {
			d = strings.TrimPrefix(d, "*.")

			registrable, err := domain.ExtractRegistrable(d)
			if err != nil {
				continue
			}

			if _, ok := seen[registrable]; ok {
				continue
			}
			seen[registrable] = struct{}{}

			if len(seen) > 100_000 {
				seen = make(map[string]struct{})
			}

			handler(ctx, registrable)
		}
	}
}
