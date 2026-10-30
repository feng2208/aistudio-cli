package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aistudio-cli/internal/aistudio"
)

type fakeServeService struct {
	models []aistudio.Model
}

func (service *fakeServeService) Models(context.Context) ([]aistudio.Model, error) {
	return append([]aistudio.Model(nil), service.models...), nil
}

func (service *fakeServeService) CountTokens(context.Context, aistudio.TokenCountRequest) (aistudio.TokenCount, error) {
	return aistudio.TokenCount{}, nil
}

func (service *fakeServeService) Generate(ctx context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	events := make(chan aistudio.Event, 2)
	events <- aistudio.Event{Kind: aistudio.EventText, Part: aistudio.Part{Text: "hello"}}
	events <- aistudio.Event{Kind: aistudio.EventFinish, FinishReason: "stop"}
	close(events)
	return events, nil
}

func newTestServeHandler() http.Handler {
	return newPublicAPIHandler(&fakeServeService{
		models: []aistudio.Model{{
			ID:      "test-model",
			Name:    "Test Model",
			Methods: []string{"generateContent"},
		}},
	}, "test-key")
}

func serveRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer test-key")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestServeModelsRequiresAPIKey(t *testing.T) {
	handler := newTestServeHandler()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without API key, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, serveRequest(http.MethodGet, "/v1/models", ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 with API key, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "test-model" {
		t.Fatalf("unexpected models payload: %s", recorder.Body.String())
	}
}

func TestServeChatCompletionsNonStream(t *testing.T) {
	handler := newTestServeHandler()
	body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}],"stream":false}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, serveRequest(http.MethodPost, "/v1/chat/completions", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if len(payload.Choices) != 1 || payload.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected chat payload: %s", recorder.Body.String())
	}
	if payload.Choices[0].FinishReason != "stop" {
		t.Fatalf("unexpected finish reason: %s", recorder.Body.String())
	}
}

func TestServeChatCompletionsStream(t *testing.T) {
	handler := newTestServeHandler()
	body := `{"model":"test-model","messages":[{"role":"user","content":"hi"}],"stream":true}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, serveRequest(http.MethodPost, "/v1/chat/completions", body))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	output := recorder.Body.String()
	if !strings.Contains(output, "hello") {
		t.Fatalf("missing streamed text: %s", output)
	}
	if !strings.Contains(output, "[DONE]") {
		t.Fatalf("missing stream terminator: %s", output)
	}
}

func TestServeDoesNotExposeAdminRoutes(t *testing.T) {
	handler := newTestServeHandler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, serveRequest(http.MethodGet, "/api/status", ""))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected admin route to be unavailable, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
