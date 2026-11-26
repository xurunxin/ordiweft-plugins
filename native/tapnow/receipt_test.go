package tapnow_test

import (
	"sync"
	"testing"

	"gitea.nkit.top/xurx/Ordiweft/internal/tapnow"
)

func TestReceiptConcurrentReadAndReplace(t *testing.T) {
	c := tapnow.New("", t.TempDir())
	const id = "concurrent-receipt"
	r := tapnow.Receipt{Owner: "fixture", Phase: "pending", Ticket: tapnow.Ticket{ProjectID: "fixture-project", NodeIDs: []string{"node"}}}
	if err := c.SaveReceipt(id, r); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if got, ok := c.Receipt(id); !ok || got.Owner != r.Owner || got.Ticket.ProjectID != r.Ticket.ProjectID {
					t.Error("concurrent read lost the durable receipt")
					return
				}
			}
		}()
	}
	defer func() { close(stop); readers.Wait() }()
	for range 200 {
		if err := c.SaveReceipt(id, r); err != nil {
			t.Fatalf("replace while querying receipt: %v", err)
		}
	}
}
