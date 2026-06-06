package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGetWeatherWithClient(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/北京" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"current_condition": [{
					"weatherDesc": [{"value": "Sunny"}],
					"temp_C": "26"
				}]
			}`)),
			Header: make(http.Header),
		}, nil
	})}

	got, err := getWeatherWithClient(
		context.Background(),
		client,
		"http://weather.test/%s?format=j1",
		"北京",
	)
	if err != nil {
		t.Fatalf("getWeatherWithClient returned error: %v", err)
	}

	want := "北京当前天气：Sunny，气温26摄氏度"
	if got != want {
		t.Fatalf("getWeatherWithClient() = %q, want %q", got, want)
	}
}

func TestParseWeatherArgs(t *testing.T) {
	city, err := parseWeatherArgs(`{"city":"上海"}`)
	if err != nil {
		t.Fatalf("parseWeatherArgs returned error: %v", err)
	}
	if city != "上海" {
		t.Fatalf("parseWeatherArgs() = %q, want %q", city, "上海")
	}
}

func TestContinueAgentLoop(t *testing.T) {
	tests := []struct {
		name      string
		toolCalls int
		round     int
		want      bool
	}{
		{name: "continue when model requests a tool", toolCalls: 1, round: 0, want: true},
		{name: "stop when model returns an answer", toolCalls: 0, round: 0, want: false},
		{name: "stop at maximum rounds", toolCalls: 1, round: maxAgentRounds - 1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := continueAgentLoop(tt.toolCalls, tt.round); got != tt.want {
				t.Fatalf("continueAgentLoop(%d, %d) = %v, want %v", tt.toolCalls, tt.round, got, tt.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
