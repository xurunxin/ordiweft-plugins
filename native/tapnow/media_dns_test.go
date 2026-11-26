package tapnow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncryptedMediaDNSRefreshAndPublicOnly(t *testing.T) {
	address := "8.8.8.8"
	status := 0
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("name") != "media.example" || r.URL.Query().Get("type") != "A" || r.Header.Get("Accept") != "application/dns-json" {
			t.Error("DNS query contract")
		}
		fmt.Fprintf(w, `{"Status":%d,"Answer":[{"type":1,"data":%q}]}`, status, address)
	}))
	defer s.Close()
	old := http.DefaultTransport
	http.DefaultTransport = s.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })
	t.Setenv("IMGCANVAS_MEDIA_DNS_URL", s.URL)
	for _, ip := range []string{"8.8.8.8", "1.1.1.1"} {
		address = ip
		got, e := lookupMediaIPs(context.Background(), "media.example")
		if e != nil || got[0].IP.String() != ip {
			t.Fatal(got, e)
		}
	}
	if calls != 2 {
		t.Fatal("cached stale address", calls)
	}
	for _, ip := range []string{"198.18.1.2", "100.117.176.39", "127.0.0.1", "10.0.0.1", "::1", "not-an-ip"} {
		address = ip
		if _, e := lookupMediaIPs(context.Background(), "media.example"); e == nil {
			t.Fatal("accepted private/Fake-IP", ip)
		}
	}
	status = 3
	address = "8.8.8.8"
	if _, e := lookupMediaIPs(context.Background(), "media.example"); e == nil {
		t.Fatal("DNS error fell back")
	}
	for _, bad := range []string{"http://resolver.example/dns-query", "https://user:pass@resolver.example/dns-query", "https://resolver.example/dns-query?x=y"} {
		t.Setenv("IMGCANVAS_MEDIA_DNS_URL", bad)
		if _, e := lookupMediaIPs(context.Background(), "media.example"); e == nil {
			t.Fatal("invalid resolver", bad)
		}
	}
}
