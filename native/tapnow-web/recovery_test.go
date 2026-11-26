package tapnowweb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPollingReadInterruptionsDoNotRepeatGeneration(t *testing.T) {
	for _, failure := range []string{"network", "503", "body", "remote-write"} {
		t.Run(failure, func(t *testing.T) {
			c, paid, _ := fixture(t, "2", false)
			transport := c.http.Transport
			if transport == nil {
				transport = http.DefaultTransport
			}
			reads := 0
			c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/generations/tasks") {
					reads++
					if reads == 1 && failure != "remote-write" {
						if failure == "network" {
							return nil, errors.New("simulated disconnect")
						}
						if failure == "503" {
							return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
						}
						return &http.Response{StatusCode: 200, Body: &brokenRead{}, Header: make(http.Header)}, nil
					}
				}
				if failure == "remote-write" && reads > 0 && r.Method == "POST" {
					return nil, errors.New("remote presentation write failed")
				}
				return transport.RoundTrip(r)
			})
			_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
			media, _, err := c.Produce(context.Background(), "recover-test-001", "alice", "fixture-local", Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 3}, nil, nil)
			if err != nil || len(media) != 1 || paid.Load() != 1 {
				t.Fatalf("media=%v err=%v paid=%d", media, err, paid.Load())
			}
			if failure != "remote-write" && reads != 2 {
				t.Fatalf("reads=%d", reads)
			}
		})
	}
}

type brokenRead struct{}

func (*brokenRead) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (*brokenRead) Close() error             { return nil }

func TestStalledPollingExitsWithOriginalTicket(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	readAttempts := 0
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/generations/tasks") {
			readAttempts++
			return nil, errors.New("simulated sustained disconnect")
		}
		return transport.RoundTrip(r)
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	timer := time.AfterFunc(5*time.Second, cancel)
	defer timer.Stop()
	_, ticket, err := c.Produce(ctx, "stalled-poll", "alice", "fixture-local", Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 3}, nil, nil)
	receipt, ok := c.Receipt("stalled-poll")
	if !errors.Is(err, errReadTemporary) || !ok || receipt.Phase != "pending" || len(ticket.TaskIDs) != 1 || paid.Load() != 1 || readAttempts != 8 {
		t.Fatalf("receipt=%+v ticket=%v error=%v paid=%d reads=%d", receipt, ticket, err, paid.Load(), readAttempts)
	}
}

func TestPollingTimeoutKeepsTicketInsteadOfInventingFailure(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	c.Timeout = 2 * time.Second
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/generations/tasks") {
			return nil, errors.New("temporary network failure")
		}
		return transport.RoundTrip(r)
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	// Give durable preparation time to finish on slower disks; the timeout
	// assertion concerns polling after a ticket exists, not submission latency.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, ticket, err := c.Produce(ctx, "recover-timeout", "alice", "fixture-local", Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 3}, nil, nil)
	r, ok := c.Receipt("recover-timeout")
	if !errors.Is(err, context.DeadlineExceeded) || !ok || r.Phase != "pending" || len(ticket.TaskIDs) != 1 || paid.Load() != 1 {
		t.Fatalf("%+v %v %v paid=%d", r, ticket, err, paid.Load())
	}
}

func TestRemoteQueueDoesNotInheritPreparationDeadline(t *testing.T) {
	c, paid, _ := fixture(t, "2", false)
	transport := c.http.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	reads := 0
	c.http.Transport = observeTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/generations/tasks") {
			reads++
			if _, hasDeadline := r.Context().Deadline(); hasDeadline {
				t.Error("saved task polling inherited preparation timeout")
			}
			if reads < 4 {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":[{"id":"fixture-task","status":"queued"}]}`))}, nil
			}
		}
		return transport.RoundTrip(r)
	})
	_, mode, _ := c.catalog.Find("nano-banana-flash", "image", "text_to_image")
	media, ticket, err := c.Produce(context.Background(), "queue-wait-once", "alice", "fixture-local", Request{Model: "nano-banana-flash", Mode: mode.ID, Kind: "image", Prompt: "fixture", Parameters: defaults(mode), MaxCost: 3}, nil, nil)
	if err != nil || len(media) != 1 || reads != 4 || len(ticket.TaskIDs) != 1 || paid.Load() != 1 {
		t.Fatal(media, ticket, err, reads, paid.Load())
	}
}
