package main

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"
	"math/big"
)

func TestFastMiningSubmitID(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"numeric_id_first", `{"id":1,"method":"mining.submit","params":[]}`, "1", true},
		{"numeric_id_last", `{"method":"mining.submit","params":[],"id":42}`, "42", true},
		{"string_id", `{"id":"abc","method":"mining.submit","params":[]}`, `"abc"`, true},
		{"null_id", `{"id":null,"method":"mining.submit","params":[]}`, "null", true},
		{"with_spaces", "{ \"method\" : \"mining.submit\" , \"id\" : 7 , \"params\" : [] }", "7", true},
		{"not_submit", `{"id":1,"method":"mining.ping","params":[]}`, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := fastMiningSubmitID([]byte(tt.in))
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if string(id) != tt.want {
				t.Fatalf("id=%q want %q", string(id), tt.want)
			}
		})
	}
}

type writeCaptureConn struct{ bytes.Buffer }

func (c *writeCaptureConn) Read([]byte) (int, error) { return 0, nil }
func (c *writeCaptureConn) Write(b []byte) (int, error) {
	return c.Buffer.Write(b)
}
func (c *writeCaptureConn) Close() error                     { return nil }
func (c *writeCaptureConn) LocalAddr() net.Addr              { return &net.IPAddr{} }
func (c *writeCaptureConn) RemoteAddr() net.Addr             { return &net.IPAddr{} }
func (c *writeCaptureConn) SetDeadline(time.Time) error      { return nil }
func (c *writeCaptureConn) SetReadDeadline(time.Time) error  { return nil }
func (c *writeCaptureConn) SetWriteDeadline(time.Time) error { return nil }

func TestProcessSubmissionTaskFastAckSilentReject(t *testing.T) {
	conn := &writeCaptureConn{}
	mc := &MinerConn{
		id:         "test",
		conn:       conn,
		writer:     bufio.NewWriter(conn),
		authorized: true,
		activeJobs: map[string]*Job{},
	}
	mc.stats.Worker = "worker1"

	job := &Job{
		JobID: "job1",
		Template: GetBlockTemplateResult{
			Height:   1,
			CurTime:  1700000000,
			Bits:     "1d00ffff",
			Previous: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		Extranonce2Size:         4,
		TemplateExtraNonce2Size: 8,
	}
	mc.activeJobs["job1"] = job

	// Invalid extranonce2 length triggers silent rejection in the fast-ack path.
	raw := []byte(`{"id":1,"method":"mining.submit","params":["worker1","job1","00","00000000","00000000"]}`)
	mc.processSubmissionTask(submissionTask{
		mc:        mc,
		rawLine:   raw,
		receivedAt: time.Now(),
		optimistic: true,
	})

	if conn.Len() != 0 {
		t.Fatalf("expected no writes for silent reject, got %q", conn.String())
	}
}

func TestProcessSubmissionTaskFastAckWinningBlockStillSubmits(t *testing.T) {
	// Make any computed header hash count as a "block" by using the maximum target.
	maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	trpc := &timingRPC{}
	conn := &writeCaptureConn{}
	mc := &MinerConn{
		id:         "test",
		conn:       conn,
		writer:     bufio.NewWriter(conn),
		rpc:        trpc,
		authorized: true,
		activeJobs: map[string]*Job{},
		extranonce1: []byte{0x01, 0x02, 0x03, 0x04},
		cfg:        Config{PoolFeePercent: 0},
	}
	mc.stats.Worker = "worker1"

	job := &Job{
		JobID: "job1",
		Template: GetBlockTemplateResult{
			Height:        101,
			CurTime:       1700000000,
			Bits:          "1d00ffff",
			Version:       1,
			Previous:      "0000000000000000000000000000000000000000000000000000000000000000",
			CoinbaseValue: 50 * 1e8,
		},
		Target:                  maxTarget,
		Extranonce2Size:         4,
		TemplateExtraNonce2Size: 8,
		PayoutScript:            []byte{0x51}, // OP_TRUE
		WitnessCommitment:       "",
		CoinbaseMsg:             "goPool-fastpath-test",
		ScriptTime:              0,
		Transactions:            nil,
		MerkleBranches:          nil,
		CoinbaseValue:           50 * 1e8,
	}
	mc.activeJobs["job1"] = job

	// Valid shapes (extranonce2 must be 4 bytes = 8 hex chars).
	raw := []byte(`{"id":1,"method":"mining.submit","params":["worker1","job1","aabbccdd","6553f100","00000001"]}`)
	trpc.start = time.Now()
	mc.processSubmissionTask(submissionTask{
		mc:         mc,
		rawLine:    raw,
		receivedAt: time.Now(),
		optimistic: true,
	})

	if trpc.method != "submitblock" {
		t.Fatalf("expected submitblock to be called, got %q", trpc.method)
	}
	if trpc.elapsed <= 0 {
		t.Fatalf("expected positive elapsed time, got %s", trpc.elapsed)
	}
	// Optimistic paths must not send any follow-up response.
	if conn.Len() != 0 {
		t.Fatalf("expected no writes during optimistic winning block processing, got %q", conn.String())
	}
}
