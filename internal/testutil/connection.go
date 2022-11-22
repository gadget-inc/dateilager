package testutil

import (
	"context"
	"fmt"
	"os"
	"time"

	dlc "github.com/gadget-inc/dateilager/pkg/client"
)

type AttemptResult int

const (
	Ok AttemptResult = iota
	TimedOut
	Error
)

type Results struct {
	count         int
	failCount     int
	downtimeStart *time.Time
	downtimes     []time.Duration
}

func (r *Results) Add(attempt AttemptResult) {
	now := time.Now()

	r.count += 1
	if attempt != Ok {
		r.failCount += 1
	}

	switch {
	case r.downtimeStart == nil && attempt != Ok:
		r.downtimeStart = &now
	case r.downtimeStart != nil && attempt == Ok:
		r.downtimes = append(r.downtimes, now.Sub(*r.downtimeStart))
		r.downtimeStart = nil
	}
}

func (r *Results) Summarize() {
	var max time.Duration
	for _, duration := range r.downtimes {
		if duration > max {
			max = duration
		}
	}

	fmt.Println("--- Result ---")
	fmt.Printf("request count: %d\n", r.count)
	fmt.Printf("success rate:  %2.f%%\n", float32(r.count-r.failCount)/float32(r.count)*100)
	fmt.Printf("max downtime:  %s\n", max.String())
}

func tryConnect(ctx context.Context, client *dlc.Client, timeout time.Duration) AttemptResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := client.Get(ctx, 1, "a", nil, dlc.VersionRange{})
	if os.IsTimeout(err) {
		fmt.Printf("conn timed out: %v\n", err)
		return TimedOut
	}
	if err != nil {
		fmt.Printf("conn error:     %v\n", err)
		return Error
	}
	return Ok
}

func TestConnection(ctx context.Context, client *dlc.Client) error {
	results := Results{}

	clock := time.NewTicker(100 * time.Millisecond)
	defer clock.Stop()

	for {
		select {
		case <-ctx.Done():
			results.Summarize()
			return nil
		case <-clock.C:
			results.Add(tryConnect(ctx, client, 50*time.Millisecond))
		}
	}
}
