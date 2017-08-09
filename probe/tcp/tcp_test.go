package tcp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"jiabiao/common/probe"
)

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func TestTcpHealthChecker(t *testing.T) {
	// Setup a test server that responds to probing correctly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	tHost, tPortStr, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	tPort, err := strconv.Atoi(tPortStr)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	tests := []struct {
		host string
		port int

		expectedStatus probe.Result
		expectedError  error
		// Some errors are different depending on your system.
		// The test passes as long as the output matches one of them.
		expectedOutputs []string
	}{
		// A connection is made and probing would succeed
		{tHost, tPort, probe.Success, nil, []string{""}},
		// No connection can be made and probing would fail
		{tHost, -1, probe.Failure, nil, []string{
			"unknown port",
			"Servname not supported for ai_socktype",
			"nodename nor servname provided, or not known",
			"dial tcp: invalid port",
		}},
	}

	prober := New()
	for i, tt := range tests {
		status, output, err := prober.Probe(tt.host, tt.port, 1*time.Second)
		if status != tt.expectedStatus {
			t.Errorf("#%d: expected status=%v, get=%v", i, tt.expectedStatus, status)
		}
		if err != tt.expectedError {
			t.Errorf("#%d: expected error=%v, get=%v", i, tt.expectedError, err)
		}
		dat := ""
		if output != nil {
			dat = ioutils.ReadAll(output)
		}
		if !containsAny(output, tt.expectedOutputs) {
			t.Errorf("#%d: expected output=one of %#v, get=%s", i, tt.expectedOutputs, output)
		}
	}
}
