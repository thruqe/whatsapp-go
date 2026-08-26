package cliutils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryWhy_Success(t *testing.T) {
	expectedQuery := "what makes aging impossible to reverse"
	expectedAnswer := "people die when aging, disease, injury, or vital-system failure pushes the body past repair; death is biology’s hard limit."
	expectedPrompt := "trace the damage that keeps accumulating"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Origin") != "https://why.com" {
			t.Errorf("expected Origin https://why.com, got %s", r.Header.Get("Origin"))
		}
		if r.Header.Get("Referer") != "https://why.com/" {
			t.Errorf("expected Referer https://why.com/, got %s", r.Header.Get("Referer"))
		}

		var req WhyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Action != "answer" {
			t.Errorf("expected action 'answer', got %q", req.Action)
		}
		if req.Query != expectedQuery {
			t.Errorf("expected query %q, got %q", expectedQuery, req.Query)
		}

		resp := WhyResponse{
			Answer: expectedAnswer,
			Prompt: expectedPrompt,
			Pulls: []WhyPull{
				{
					Role:      "deepen",
					Lens:      "cost",
					Heat:      8,
					Label:     "how does cellular wear spread?",
					Query:     "how does cellular wear spread across the body?",
					Anchor:    "cellular wear",
					Gap:       "how wear propagates between tissues",
					Grounding: "required",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalURL := WhyAPIURL
	WhyAPIURL = server.URL
	defer func() { WhyAPIURL = originalURL }()

	res, err := QueryWhy(context.Background(), expectedQuery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Answer != expectedAnswer {
		t.Errorf("expected answer %q, got %q", expectedAnswer, res.Answer)
	}
	if res.Prompt != expectedPrompt {
		t.Errorf("expected prompt %q, got %q", expectedPrompt, res.Prompt)
	}
	if len(res.Pulls) != 1 {
		t.Fatalf("expected 1 pull, got %d", len(res.Pulls))
	}
	if res.Pulls[0].Label != "how does cellular wear spread?" {
		t.Errorf("unexpected pull label: %s", res.Pulls[0].Label)
	}
}

func TestQueryWhy_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unknown API action."})
	}))
	defer server.Close()

	originalURL := WhyAPIURL
	WhyAPIURL = server.URL
	defer func() { WhyAPIURL = originalURL }()

	_, err := QueryWhy(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
