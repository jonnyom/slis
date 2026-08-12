package session

import (
	"context"
	"testing"
	"time"
)

func TestRequestFrontendFocusReceivesMatchingAcknowledgement(t *testing.T) {
	directory := t.TempDir()
	request := FocusRequest{ID: "request-1", GroupID: "feature", TabID: "agent", TimeNS: time.Now().UnixNano()}
	read := make(chan FocusRequest, 1)
	go func() {
		for {
			current, err := ReadFocusRequest(directory)
			if err == nil {
				read <- current
				_ = WriteFocusAck(directory, current.ID)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	focused, err := RequestFrontendFocus(ctx, directory, request)
	if err != nil {
		t.Fatal(err)
	}
	if !focused {
		t.Fatal("frontend did not acknowledge focus")
	}
	if got := <-read; got != request {
		t.Fatalf("request = %#v, want %#v", got, request)
	}
}

func TestRequestFrontendFocusTimesOutWithoutFrontend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	focused, err := RequestFrontendFocus(ctx, t.TempDir(), FocusRequest{ID: "request-1", GroupID: "feature", TabID: "agent", TimeNS: time.Now().UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if focused {
		t.Fatal("focus succeeded without a frontend")
	}
}

func TestFocusRequestFresh(t *testing.T) {
	now := time.Now()
	if !(FocusRequest{TimeNS: now.Add(-time.Second).UnixNano()}).Fresh(now) {
		t.Fatal("recent request is stale")
	}
	if (FocusRequest{TimeNS: now.Add(-time.Minute).UnixNano()}).Fresh(now) {
		t.Fatal("old request is fresh")
	}
}
