package outbox

import (
	"testing"
	"time"
)

func TestSanitizeConfig(t *testing.T) {
	cfg := sanitizeConfig(Config{})
	if cfg.BatchSize != defaultBatchSize {
		t.Fatalf("expected default batch size, got %d", cfg.BatchSize)
	}
	if cfg.TickInterval != defaultTickInterval {
		t.Fatalf("expected default tick interval, got %v", cfg.TickInterval)
	}
	if cfg.InitialBackoff != defaultInitialBackoff {
		t.Fatalf("expected default initial backoff, got %v", cfg.InitialBackoff)
	}
	if cfg.MaxBackoff != defaultMaxBackoff {
		t.Fatalf("expected default max backoff, got %v", cfg.MaxBackoff)
	}
	if cfg.MaxAttempts != defaultMaxAttempts {
		t.Fatalf("expected default max attempts, got %d", cfg.MaxAttempts)
	}
	if cfg.PublishTimeout != defaultPublishTimeout {
		t.Fatalf("expected default publish timeout, got %v", cfg.PublishTimeout)
	}
	if cfg.Workers != defaultWorkers {
		t.Fatalf("expected default workers, got %d", cfg.Workers)
	}
}

func TestBackoffDuration(t *testing.T) {
	task := &PublisherTask{cfg: sanitizeConfig(Config{}), clock: time.Now}
	if got := task.backoffDuration(0); got != defaultInitialBackoff {
		t.Fatalf("attempt 0 expected %v, got %v", defaultInitialBackoff, got)
	}
	if got := task.backoffDuration(3); got != defaultInitialBackoff*8 {
		t.Fatalf("attempt 3 expected %v, got %v", defaultInitialBackoff*8, got)
	}
	task.cfg.MaxBackoff = 5 * time.Second
	if got := task.backoffDuration(10); got != 5*time.Second {
		t.Fatalf("expected capped backoff 5s, got %v", got)
	}
}
