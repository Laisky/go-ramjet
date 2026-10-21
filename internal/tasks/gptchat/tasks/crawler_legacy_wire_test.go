package tasks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// legacyCrawlerTaskD58d6c75 freezes library/db/redis/models.go at GraphQL
// d58d6c75a705, before optional failure diagnostics and acquisition metadata.
// Keep this historical shape independent of the current dependency's DTO.
type legacyCrawlerTaskD58d6c75 struct {
	TaskID         string                        `json:"task_id"`
	CreatedAt      time.Time                     `json:"created_at"`
	Status         string                        `json:"status"`
	FailedReason   *string                       `json:"failed_reason,omitempty"`
	FinishedAt     *time.Time                    `json:"finished_at,omitempty"`
	Url            string                        `json:"url"`
	APIKey         string                        `json:"api_key,omitempty"`
	OutputMarkdown bool                          `json:"output_markdown,omitempty"`
	Egress         *legacyCrawlerPolicyD58d6c75  `json:"egress,omitempty"`
	RequestChain   []string                      `json:"request_chain,omitempty"`
	EgressReceipt  *legacyCrawlerReceiptD58d6c75 `json:"egress_receipt,omitempty"`
	ResultHTML     []byte                        `json:"result_html,omitempty"`
	ResultMarkdown []byte                        `json:"result_markdown,omitempty"`
}

// legacyCrawlerPolicyD58d6c75 freezes the policy fields used by the old DTO.
type legacyCrawlerPolicyD58d6c75 struct {
	Host              string   `json:"host"`
	Addresses         []string `json:"addresses"`
	MaxRedirects      int      `json:"max_redirects"`
	AllowSubresources bool     `json:"allow_subresources"`
}

// legacyCrawlerReceiptD58d6c75 freezes the old direct connection receipt.
type legacyCrawlerReceiptD58d6c75 struct {
	Version         int                           `json:"version"`
	TaskID          string                        `json:"task_id"`
	TargetURL       string                        `json:"target_url"`
	PolicyDigest    string                        `json:"policy_digest"`
	Origins         []legacyCrawlerOriginD58d6c75 `json:"origins"`
	BlockedRequests int64                         `json:"blocked_requests"`
}

// legacyCrawlerOriginD58d6c75 freezes each old receipt's observed connection.
type legacyCrawlerOriginD58d6c75 struct {
	URL         string   `json:"url"`
	Addresses   []string `json:"addresses"`
	PeerAddress string   `json:"peer_address"`
	Kind        string   `json:"kind"`
	BodyBytes   int64    `json:"body_bytes"`
}

// roundTripLegacyCrawlerTask decodes payload using the frozen legacy schema and
// returns the old task and JSON, asserting that only known additions are lost.
func roundTripLegacyCrawlerTask(t *testing.T, payload string) (legacyCrawlerTaskD58d6c75, string) {
	t.Helper()
	var legacy legacyCrawlerTaskD58d6c75
	require.NoError(t, json.Unmarshal([]byte(payload), &legacy))
	encoded, err := json.Marshal(legacy)
	require.NoError(t, err)
	var expected map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload), &expected))
	for _, addition := range []string{
		"failure_stage", "failure_category", "failure_signal", "failure_context",
		"allow_provider_fallback", "acquisition",
	} {
		delete(expected, addition)
	}
	oldFields, err := json.Marshal(expected)
	require.NoError(t, err)
	require.JSONEq(t, string(oldFields), string(encoded), "every historical wire field must survive the old decoder")
	return legacy, string(encoded)
}
