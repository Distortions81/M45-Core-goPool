package main

import (
	"runtime"
	"testing"
)

func TestNormalizeWorkerCount(t *testing.T) {
	if got := normalizeWorkerCount(0); got != 1 {
		t.Fatalf("normalizeWorkerCount(0) = %d, want 1", got)
	}
	if got := normalizeWorkerCount(-5); got != 1 {
		t.Fatalf("normalizeWorkerCount(-5) = %d, want 1", got)
	}
	if got := normalizeWorkerCount(3); got != 3 {
		t.Fatalf("normalizeWorkerCount(3) = %d, want 3", got)
	}
}

func TestSubmissionWorkerQueueDepth(t *testing.T) {
	// workerCount=1 should be clamped by the minimum.
	if got, want := submissionWorkerQueueDepth(1), submissionWorkerQueueMinDepth; got != want {
		t.Fatalf("submissionWorkerQueueDepth(1) = %d, want %d", got, want)
	}
	// Larger workerCount should scale by the multiplier.
	if got, want := submissionWorkerQueueDepth(10), 10*submissionWorkerQueueMultiplier; got != want {
		t.Fatalf("submissionWorkerQueueDepth(10) = %d, want %d", got, want)
	}
}

func TestDefaultSubmissionWorkerCountUsesGOMAXPROCS(t *testing.T) {
	old := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(old) })

	runtime.GOMAXPROCS(2)
	if got := defaultSubmissionWorkerCount(); got != 2 {
		t.Fatalf("defaultSubmissionWorkerCount() = %d, want 2", got)
	}
}
