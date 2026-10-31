package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"smartcity/internal/domain"
)

func TestWriteErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{domain.ErrInvalidInput("bad"), 400},
		{domain.ErrNotFound, 404},
		{fmt.Errorf("wrapped: %w", domain.ErrNotFound), 404},
		{fmt.Errorf("%w: dup", domain.ErrConflict), 409},
		{domain.ErrUnauthorized, 401},
		{domain.ErrForbidden, 403},
		{domain.ErrRateLimited, 429},
		{errors.New("db down"), 500},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, c.err)
		if rec.Code != c.want {
			t.Errorf("%v: status %d, want %d", c.err, rec.Code, c.want)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
			t.Errorf("%v: expected JSON error body, got %q", c.err, rec.Body.String())
		}
	}
	// internal errors must not leak details
	rec := httptest.NewRecorder()
	writeError(rec, errors.New("password=hunter2"))
	if rec.Body.String() != "{\"error\":\"internal error\"}\n" {
		t.Fatalf("leaked: %s", rec.Body.String())
	}
}

func TestPathID(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/buses/12", nil)
	r.SetPathValue("id", "12")
	if id, err := pathID(r, "id"); err != nil || id != 12 {
		t.Fatalf("got %d %v", id, err)
	}
	for _, v := range []string{"", "abc", "-1", "0"} {
		r.SetPathValue("id", v)
		if _, err := pathID(r, "id"); err == nil {
			t.Fatalf("%q should be invalid", v)
		}
	}
}
