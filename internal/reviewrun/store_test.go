package reviewrun

import (
	"sync"
	"testing"
)

func TestStorePersistsAReviewConversation(t *testing.T) {
	store := Open(t.TempDir())

	run, err := store.Create("payroll-fix", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if run.Status != StatusQueued {
		t.Fatalf("status = %q, want %q", run.Status, StatusQueued)
	}

	if _, err := store.AppendMessage(run.ID, RoleUser, "Review the stack."); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := store.SetRunning(run.ID); err != nil {
		t.Fatalf("SetRunning: %v", err)
	}
	if _, err := store.AppendMessage(run.ID, RoleReviewer, "Two changes are needed."); err != nil {
		t.Fatalf("AppendMessage reviewer: %v", err)
	}
	if err := store.SetCompleted(run.ID, 2); err != nil {
		t.Fatalf("SetCompleted: %v", err)
	}
	if err := store.SetCompleted(run.ID, 0); err != nil {
		t.Fatalf("SetCompleted follow-up: %v", err)
	}

	detail, err := store.Get(run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.Status != StatusFindings {
		t.Fatalf("status = %q, want %q", detail.Status, StatusFindings)
	}
	if detail.FindingCount != 2 {
		t.Fatalf("finding count = %d, want 2", detail.FindingCount)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(detail.Messages))
	}
	if detail.Messages[0].Role != RoleUser || detail.Messages[0].Body != "Review the stack." {
		t.Fatalf("first message = %#v", detail.Messages[0])
	}
	if detail.Messages[1].Role != RoleReviewer || detail.Messages[1].Body != "Two changes are needed." {
		t.Fatalf("second message = %#v", detail.Messages[1])
	}

	runs, err := store.List("payroll-fix")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %#v", runs)
	}

	details, err := store.ListDetails("payroll-fix")
	if err != nil {
		t.Fatalf("ListDetails: %v", err)
	}
	if len(details) != 1 || len(details[0].Messages) != 2 {
		t.Fatalf("details = %#v", details)
	}
}

func TestStoreRecordsCleanAndFailedReviews(t *testing.T) {
	store := Open(t.TempDir())

	clean, err := store.Create("clean-slice", "Claude", "review-claude")
	if err != nil {
		t.Fatalf("Create clean: %v", err)
	}
	if err := store.SetCompleted(clean.ID, 0); err != nil {
		t.Fatalf("SetCompleted: %v", err)
	}

	failed, err := store.Create("failed-slice", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := store.SetFailed(failed.ID, "review process exited"); err != nil {
		t.Fatalf("SetFailed: %v", err)
	}

	cleanDetail, err := store.Get(clean.ID)
	if err != nil {
		t.Fatalf("Get clean: %v", err)
	}
	if cleanDetail.Status != StatusClean {
		t.Fatalf("clean status = %q, want %q", cleanDetail.Status, StatusClean)
	}

	failedDetail, err := store.Get(failed.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if failedDetail.Status != StatusFailed || failedDetail.Error != "review process exited" {
		t.Fatalf("failed detail = %#v", failedDetail)
	}
}

func TestStoreRecordsACompletedRequestBeforeFinalStatus(t *testing.T) {
	store := Open(t.TempDir())
	run, err := store.Create("payroll-fix", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	message, err := store.AppendMessage(run.ID, RoleUser, "Review the stack.")
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	if err := store.SetRunning(run.ID); err != nil {
		t.Fatalf("SetRunning: %v", err)
	}
	if err := store.RecordTurn(run.ID, message.ID, 1); err != nil {
		t.Fatalf("RecordTurn: %v", err)
	}

	detail, err := store.Get(run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.LastRequest != message.ID || detail.FindingCount != 1 {
		t.Fatalf("recorded turn = %#v", detail.Run)
	}
	if detail.Status != StatusFindings {
		t.Fatalf("status = %q, want %q", detail.Status, StatusFindings)
	}
}

func TestStoreSerializesConcurrentUpdates(t *testing.T) {
	store := Open(t.TempDir())
	run, err := store.Create("payroll-fix", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const updates = 40
	start := make(chan struct{})
	errs := make(chan error, updates)
	var wait sync.WaitGroup
	for range updates {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- store.RecordTurn(run.ID, "", 1)
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for updateErr := range errs {
		if updateErr != nil {
			t.Fatalf("RecordTurn: %v", updateErr)
		}
	}

	detail, err := store.Get(run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if detail.FindingCount != updates {
		t.Fatalf("finding count = %d, want %d", detail.FindingCount, updates)
	}
}

func TestStoreResumesAStagedTurnWithoutDuplicatingTheReply(t *testing.T) {
	store := Open(t.TempDir())
	run, err := store.Create("payroll-fix", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	request, err := store.AppendMessage(run.ID, RoleUser, "Review the stack.")
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	turn := PendingTurn{
		RequestID: request.ID, Summary: "One change is needed.",
		Findings: []byte(`[{"file":"checkout.go"}]`), FindingCount: 1,
	}
	if err := store.StageTurn(run.ID, turn); err != nil {
		t.Fatalf("StageTurn: %v", err)
	}
	if _, err := store.AppendReviewerMessage(run.ID, request.ID, turn.Summary); err != nil {
		t.Fatalf("AppendReviewerMessage: %v", err)
	}
	if _, err := store.AppendReviewerMessage(run.ID, request.ID, turn.Summary); err != nil {
		t.Fatalf("AppendReviewerMessage retry: %v", err)
	}
	if err := store.MarkTurnDelivered(run.ID, turn.RequestID); err != nil {
		t.Fatalf("MarkTurnDelivered: %v", err)
	}

	staged, found, err := store.GetPendingTurn(run.ID)
	if err != nil {
		t.Fatalf("GetPendingTurn: %v", err)
	}
	if !found || !staged.DeliveryCompleted || staged.RequestID != request.ID {
		t.Fatalf("pending turn = %#v, found = %v", staged, found)
	}
	detail, err := store.Get(run.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %#v", detail.Messages)
	}

	if err := store.SetTurnCompleted(run.ID, staged.RequestID, staged.FindingCount); err != nil {
		t.Fatalf("SetTurnCompleted: %v", err)
	}
	if err := store.ClearPendingTurn(run.ID, turn.RequestID); err != nil {
		t.Fatalf("ClearPendingTurn: %v", err)
	}
	if _, found, err := store.GetPendingTurn(run.ID); err != nil || found {
		t.Fatalf("pending turn after clear found = %v, err = %v", found, err)
	}
}

func TestStoreDoesNotOverwriteAStagedTurn(t *testing.T) {
	store := Open(t.TempDir())
	run, err := store.Create("payroll-fix", "Codex", "review-codex")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	first := PendingTurn{RequestID: "request-1", Summary: "first"}
	if err := store.StageTurn(run.ID, first); err != nil {
		t.Fatalf("StageTurn first: %v", err)
	}
	if err := store.StageTurn(run.ID, PendingTurn{RequestID: "request-2", Summary: "second"}); err == nil {
		t.Fatal("expected conflicting staged turn to be rejected")
	}
	if err := store.MarkTurnDelivered(run.ID, "request-2"); err == nil {
		t.Fatal("expected stale delivery marker to be rejected")
	}
	if err := store.ClearPendingTurn(run.ID, "request-2"); err == nil {
		t.Fatal("expected stale clear to be rejected")
	}

	staged, found, err := store.GetPendingTurn(run.ID)
	if err != nil {
		t.Fatalf("GetPendingTurn: %v", err)
	}
	if !found || staged.RequestID != first.RequestID || staged.Summary != first.Summary {
		t.Fatalf("pending turn = %#v, found = %v", staged, found)
	}
}
