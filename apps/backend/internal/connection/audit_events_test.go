package connection_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/jtumidanski/Harbormaster/internal/apierror"
	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/connection"
	hmminio "github.com/jtumidanski/Harbormaster/internal/minio"
)

// newAuditedProcessor builds a connection.Processor wired to a fresh DB,
// the success-stub prober, an empty pool, and an audit.Processor sharing
// the same database.
func newAuditedProcessor(t *testing.T) (*connection.Processor, *audit.Processor, *gorm.DB) {
	t.Helper()
	gdb := newTestDB(t)
	cipher := newTestCipher(t)
	pool := hmminio.NewEmpty()
	p := connection.NewProcessor(gdb, cipher, pool)
	p.Probe = stubProbeOK
	a := audit.NewProcessor(gdb, 90*24*time.Hour)
	p.Audit = a
	return p, a, gdb
}

func loadLatestPayload(t *testing.T, a *audit.Processor, action string) (audit.Event, string) {
	t.Helper()
	events, err := audit.List(a.DB(), audit.Filter{Action: action, PageSize: 1})
	require.NoError(t, err)
	if len(events) == 0 {
		return audit.Event{}, ""
	}
	type row struct {
		PayloadSummaryJSON string `gorm:"column:payload_summary_json"`
	}
	var r row
	require.NoError(t,
		a.DB().
			Table("audit_events").
			Select("payload_summary_json").
			Where("id = ?", events[0].ID).
			Scan(&r).Error,
	)
	return events[0], r.PayloadSummaryJSON
}

func requireNoSecrets(t *testing.T, payload string) {
	t.Helper()
	lower := strings.ToLower(payload)
	require.NotContains(t, lower, "password")
	require.NotContains(t, lower, "secret_key")
	require.NotContains(t, lower, "secretkey")
}

func TestAuditEvent_ConnectionUpdateSuccess(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	ctx := context.Background()

	in := connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIA-PUBLIC-ID",
		SecretKey:   "topsecretvalue-do-not-leak",
	}
	require.NoError(t, p.Update(ctx, in, "operator", "10.0.0.1"))

	ev, payload := loadLatestPayload(t, a, audit.ActionConnectionUpdate)
	require.NotEmpty(t, payload, "expected an audit row for connection.update")
	require.Equal(t, audit.OutcomeSuccess, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "10.0.0.1", ev.SourceIP)
	require.Equal(t, "connection", ev.TargetType)
	require.Contains(t, payload, "minio.lan",
		"endpoint should be preserved under a non-sensitive key")
	require.Contains(t, payload, "tls_skip_verify")
	require.NotContains(t, payload, "topsecretvalue-do-not-leak",
		"connection.update payload leaked the secret key value")
	require.NotContains(t, payload, "AKIA-PUBLIC-ID",
		"connection.update payload leaked the access key value")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ConnectionUpdateFailure(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	// Force the probe to fail so Update returns an error.
	p.Probe = func(_ context.Context, _ connection.SubmitInput) (connection.TestResult, *apierror.Error) {
		return connection.TestResult{TCPConnect: map[string]string{"failed": "boom"}},
			apierror.New(422, "minio_unreachable", "boom")
	}

	in := connection.SubmitInput{
		EndpointURL: "https://broken.lan:9000",
		AccessKey:   "AKIA",
		SecretKey:   "supersecret-leaktest",
	}
	err := p.Update(context.Background(), in, "operator", "10.0.0.2")
	require.Error(t, err)

	ev, payload := loadLatestPayload(t, a, audit.ActionConnectionUpdate)
	require.NotEmpty(t, payload, "expected a failure audit row for connection.update")
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.NotContains(t, payload, "supersecret-leaktest")
	requireNoSecrets(t, payload)
}

func TestAuditEvent_ConnectionTestSuccessIsSilent(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	in := connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIA",
		SecretKey:   "sk",
	}
	_, ae := p.Test(context.Background(), in, "operator", "10.0.0.3")
	require.Nil(t, ae)

	events, err := audit.List(a.DB(), audit.Filter{Action: audit.ActionConnectionTest, PageSize: 10})
	require.NoError(t, err)
	require.Empty(t, events, "connection.test must NOT emit an audit row on success")
}

func TestAuditEvent_ConnectionTestFailureRecorded(t *testing.T) {
	p, a, _ := newAuditedProcessor(t)
	// Probe that fails on list_buckets with bad creds.
	p.Probe = func(_ context.Context, _ connection.SubmitInput) (connection.TestResult, *apierror.Error) {
		return connection.TestResult{
				TCPConnect:  "ok",
				ListBuckets: map[string]string{"failed": "InvalidAccessKeyId"},
			},
			apierror.New(422, "minio_invalid_credentials", "MinIO rejected the provided keys")
	}

	in := connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIA",
		SecretKey:   "wrong-secret-leaktest",
	}
	_, ae := p.Test(context.Background(), in, "operator", "10.0.0.4")
	require.NotNil(t, ae)

	ev, payload := loadLatestPayload(t, a, audit.ActionConnectionTest)
	require.NotEmpty(t, payload, "expected a failure audit row for connection.test")
	require.Equal(t, audit.OutcomeFailure, ev.Outcome)
	require.Equal(t, "operator", ev.Actor)
	require.Equal(t, "connection", ev.TargetType)
	require.Contains(t, payload, "tcp_connect")
	require.Contains(t, payload, "list_buckets")
	require.Contains(t, payload, "admin_ping")
	require.Contains(t, payload, "failed", "expected per-step status label in payload")
	require.NotContains(t, payload, "wrong-secret-leaktest",
		"connection.test failure payload leaked the secret key value")
	requireNoSecrets(t, payload)
}

// TestProcessor_NilAuditIsSafe verifies pre-existing tests that build the
// processor without setting .Audit still work after the wiring change.
func TestProcessor_NilAuditIsSafe(t *testing.T) {
	p, _ := newProcessor(t) // helper from processor_test.go; .Audit unset
	ctx := context.Background()

	in := connection.SubmitInput{
		EndpointURL: "https://minio.lan:9000",
		AccessKey:   "AKIA",
		SecretKey:   "sk",
	}
	require.NoError(t, p.Update(ctx, in, "", ""))
	_, ae := p.Test(ctx, in, "", "")
	require.Nil(t, ae)
}
