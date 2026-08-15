package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoverSearX(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"instances":{"https://one.test":{"status":"online","is_tor":false},"https://two.test":{"status":"offline","is_tor":false},"https://tor.test":{"status":"online","is_tor":true}}}`))
	}))
	defer ts.Close()
	got, err := DiscoverSearX(context.Background(), ts.URL, 10, ts.Client())
	if err != nil || len(got) != 1 || !strings.Contains(got[0], "one.test") {
		t.Fatalf("%v %#v", err, got)
	}
}
