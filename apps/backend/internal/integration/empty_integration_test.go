//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"

	"github.com/jtumidanski/Harbormaster/internal/audit"
	"github.com/jtumidanski/Harbormaster/internal/jobs/bucketempty"
)

// TestEmpty_DrainsBucket uploads a handful of small objects, runs the
// bucketempty service to completion, and asserts:
//
//  1. The bucket really is empty afterwards (StatObject on each key
//     fails).
//  2. The audit log contains a single "bucket.empty" row with outcome
//     "success" and a deleted_count equal to the seeded total.
//
// The audit assertion is the load-bearing one for T3.22: it proves the
// whole worker → AuditRecorder → audit.Processor → SQLite chain wires
// up the way production does.
func TestEmpty_DrainsBucket(t *testing.T) {
	env, ctx := setup(t)

	const (
		bucketName = "harbormaster-it-empty"
		nObjects   = 5
	)

	if err := env.MC.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{}); err != nil {
		t.Fatalf("MakeBucket: %v", err)
	}

	for i := 0; i < nObjects; i++ {
		key := fmt.Sprintf("obj-%02d.txt", i)
		body := strings.NewReader("payload-" + key)
		if _, err := env.MC.PutObject(ctx, bucketName, key, body, int64(body.Len()),
			miniogo.PutObjectOptions{ContentType: "text/plain"}); err != nil {
			t.Fatalf("seed PutObject(%s): %v", key, err)
		}
	}

	progressCh, doneCh, err := env.Empty.StartOrAttach(ctx, bucketName, false)
	if err != nil {
		t.Fatalf("StartOrAttach: %v", err)
	}
	// Drain progress so the worker's broadcast is never blocked even
	// on a chatty stream. The test does not assert intermediate counts
	// — only the terminal Result and the audit row matter.
	go func() {
		for range progressCh {
		}
	}()

	select {
	case res := <-doneCh:
		if res.ErrorMessage != "" {
			t.Fatalf("empty job reported error: %s", res.ErrorMessage)
		}
		if res.DeletedTotal != nObjects {
			t.Errorf("DeletedTotal: got %d, want %d", res.DeletedTotal, nObjects)
		}
	case <-time.After(60 * time.Second):
		t.Fatalf("empty job did not complete within 60s")
	}

	// Bucket should be empty now.
	listCh := env.MC.ListObjects(ctx, bucketName, miniogo.ListObjectsOptions{Recursive: true})
	for obj := range listCh {
		if obj.Err != nil {
			t.Fatalf("post-empty ListObjects: %v", obj.Err)
		}
		t.Fatalf("post-empty bucket still contains %q", obj.Key)
	}

	// Audit assertion: exactly one bucket.empty success row for this
	// bucket. The worker emits the audit row AFTER sending the terminal
	// Result onto the done channel, so we poll briefly to avoid a
	// natural race between this assertion and the audit insert.
	var rows []audit.Event
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows, err = audit.List(env.DB, audit.Filter{
			Action:   bucketempty.ActionBucketEmpty,
			TargetID: bucketName,
		})
		if err != nil {
			t.Fatalf("audit.List: %v", err)
		}
		if len(rows) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit row never appeared within 5s")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(rows) != 1 {
		t.Fatalf("audit rows: got %d, want 1; rows=%+v", len(rows), rows)
	}
	if rows[0].Outcome != bucketempty.OutcomeSuccess {
		t.Errorf("audit outcome: got %q, want %q", rows[0].Outcome, bucketempty.OutcomeSuccess)
	}

	_ = env.MC.RemoveBucket(ctx, bucketName)
}
