package main

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	// submissionWorkerQueueMultiplier determines how much backlog we allow
	// per worker goroutine.
	submissionWorkerQueueMultiplier = 32
	// submissionWorkerQueueMinDepth ensures the queue can hold at least this
	// many tasks regardless of CPU count.
	submissionWorkerQueueMinDepth = 128
)

var (
	submissionWorkers    *submissionWorkerPool
	submissionWorkerOnce sync.Once
)

func ensureSubmissionWorkerPool() {
	submissionWorkerOnce.Do(func() {
		submissionWorkers = newSubmissionWorkerPool(defaultSubmissionWorkerCount())
	})
}

func defaultSubmissionWorkerCount() int {
	// Prefer GOMAXPROCS (actual parallelism) over NumCPU (hardware threads).
	if n := runtime.GOMAXPROCS(0); n > 0 {
		return n
	}
	return 1
}

type submissionTask struct {
	mc               *MinerConn
	rawLine          []byte
	reqID            interface{}
	job              *Job
	jobID            string
	workerName       string
	extranonce2      string
	extranonce2Bytes []byte
	ntime            string
	nonce            string
	submittedVersion uint32
	versionHex       string
	useVersion       uint32
	scriptTime       int64
	policyReject     submitPolicyReject
	receivedAt       time.Time
	optimistic       bool // Response already sent; skip sending in processSubmissionTask
}

type submitPolicyReject struct {
	reason  submitRejectReason
	errCode int
	errMsg  string
}

type submissionWorkerPool struct {
	tasks chan submissionTask
}

func newSubmissionWorkerPool(workerCount int) *submissionWorkerPool {
	workerCount = normalizeWorkerCount(workerCount)
	queueDepth := submissionWorkerQueueDepth(workerCount)
	pool := &submissionWorkerPool{
		tasks: make(chan submissionTask, queueDepth),
	}
	for i := 0; i < workerCount; i++ {
		go pool.worker(i)
	}
	return pool
}

func (p *submissionWorkerPool) submit(task submissionTask) {
	p.tasks <- task
}

func (p *submissionWorkerPool) worker(id int) {
	for task := range p.tasks {
		p.processTask(id, task)
	}
}

func (p *submissionWorkerPool) processTask(id int, task submissionTask) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("submission worker panic", "worker", id, "error", r, "stack", string(debug.Stack()))
		}
	}()
	task.mc.processSubmissionTask(task)
}

func normalizeWorkerCount(workerCount int) int {
	if workerCount <= 0 {
		return 1
	}
	return workerCount
}

func submissionWorkerQueueDepth(workerCount int) int {
	queueDepth := workerCount * submissionWorkerQueueMultiplier
	if queueDepth < submissionWorkerQueueMinDepth {
		queueDepth = submissionWorkerQueueMinDepth
	}
	return queueDepth
}
