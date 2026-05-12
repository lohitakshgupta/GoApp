package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Processor interface {
	Process(ctx context.Context, j Job) (string, error)
}

type DemoProcessor struct {
	logger *slog.Logger
}

func NewDemoProcessor(logger *slog.Logger) *DemoProcessor {
	return &DemoProcessor{logger: logger}
}

func (p *DemoProcessor) Process(ctx context.Context, j Job) (string, error) {
	var payload struct {
		Message        string `json:"message"`
		DurationMillis int    `json:"duration_ms"`
		FailUntil      int    `json:"fail_until_attempt"`
	}
	if len(j.Payload) > 0 {
		if err := json.Unmarshal(j.Payload, &payload); err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
	}

	switch j.Type {
	case "echo":
		if payload.Message == "" {
			payload.Message = "ok"
		}
		return payload.Message, nil
	case "sleep":
		if payload.DurationMillis <= 0 {
			payload.DurationMillis = 250
		}
		timer := time.NewTimer(time.Duration(payload.DurationMillis) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			return fmt.Sprintf("slept for %dms", payload.DurationMillis), nil
		}
	case "flaky":
		if payload.FailUntil <= 0 {
			payload.FailUntil = 1
		}
		if j.Attempts <= payload.FailUntil {
			return "", fmt.Errorf("intentional transient failure on attempt %d", j.Attempts)
		}
		return "recovered after retry", nil
	case "fail":
		return "", errors.New("intentional failure")
	default:
		return "", fmt.Errorf("unsupported job type %q", j.Type)
	}
}
