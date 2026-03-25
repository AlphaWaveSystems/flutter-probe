package runner

import (
	"fmt"
	"testing"
)

func TestDistributeFiles_RoundRobin(t *testing.T) {
	files := []string{"a.probe", "b.probe", "c.probe", "d.probe", "e.probe", "f.probe", "g.probe"}
	buckets := DistributeFiles(files, 3)

	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}
	if len(buckets[0]) != 3 {
		t.Errorf("bucket 0: expected 3 files, got %d", len(buckets[0]))
	}
	if len(buckets[1]) != 2 {
		t.Errorf("bucket 1: expected 2 files, got %d", len(buckets[1]))
	}
	if len(buckets[2]) != 2 {
		t.Errorf("bucket 2: expected 2 files, got %d", len(buckets[2]))
	}
	// Check round-robin assignment
	if buckets[0][0] != "a.probe" || buckets[1][0] != "b.probe" || buckets[2][0] != "c.probe" {
		t.Error("round-robin order incorrect")
	}
}

func TestDistributeFiles_SingleDevice(t *testing.T) {
	files := []string{"a.probe", "b.probe", "c.probe"}
	buckets := DistributeFiles(files, 1)

	if len(buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(buckets))
	}
	if len(buckets[0]) != 3 {
		t.Errorf("expected 3 files, got %d", len(buckets[0]))
	}
}

func TestDistributeFiles_MoreDevicesThanFiles(t *testing.T) {
	files := []string{"a.probe", "b.probe"}
	buckets := DistributeFiles(files, 5)

	if len(buckets) != 5 {
		t.Fatalf("expected 5 buckets, got %d", len(buckets))
	}
	nonEmpty := 0
	for _, b := range buckets {
		if len(b) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty != 2 {
		t.Errorf("expected 2 non-empty buckets, got %d", nonEmpty)
	}
}

func TestDistributeFiles_Zero(t *testing.T) {
	buckets := DistributeFiles(nil, 3)
	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(buckets))
	}
	for _, b := range buckets {
		if len(b) != 0 {
			t.Error("expected empty buckets for nil input")
		}
	}
}

func TestShardFiles_Deterministic(t *testing.T) {
	files := []string{"a.probe", "b.probe", "c.probe", "d.probe", "e.probe", "f.probe"}

	// Get all 3 shards
	shard0 := ShardFiles(files, 0, 3)
	shard1 := ShardFiles(files, 1, 3)
	shard2 := ShardFiles(files, 2, 3)

	// Every file should be in exactly one shard
	total := len(shard0) + len(shard1) + len(shard2)
	if total != len(files) {
		t.Errorf("total across shards: %d, expected %d", total, len(files))
	}

	// Run again — should produce same assignment
	shard0b := ShardFiles(files, 0, 3)
	if len(shard0) != len(shard0b) {
		t.Error("shard assignment is not deterministic")
	}
	for i := range shard0 {
		if shard0[i] != shard0b[i] {
			t.Errorf("shard 0 mismatch at %d: %q vs %q", i, shard0[i], shard0b[i])
		}
	}
}

func TestShardFiles_SingleShard(t *testing.T) {
	files := []string{"a.probe", "b.probe"}
	result := ShardFiles(files, 0, 1)
	if len(result) != 2 {
		t.Errorf("single shard should return all files, got %d", len(result))
	}
}

func TestParseShard_Valid(t *testing.T) {
	tests := []struct {
		input     string
		wantIdx   int
		wantTotal int
	}{
		{"1/3", 0, 3},
		{"2/3", 1, 3},
		{"3/3", 2, 3},
		{"1/1", 0, 1},
		{"5/10", 4, 10},
	}
	for _, tt := range tests {
		idx, total, err := ParseShard(tt.input)
		if err != nil {
			t.Errorf("ParseShard(%q): %v", tt.input, err)
			continue
		}
		if idx != tt.wantIdx || total != tt.wantTotal {
			t.Errorf("ParseShard(%q) = (%d, %d), want (%d, %d)", tt.input, idx, total, tt.wantIdx, tt.wantTotal)
		}
	}
}

func TestParseShard_Invalid(t *testing.T) {
	invalids := []string{"0/3", "4/3", "abc", "1", "1/0"}
	for _, s := range invalids {
		_, _, err := ParseShard(s)
		if err == nil {
			t.Errorf("ParseShard(%q) should fail", s)
		}
	}
}

func TestParseShard_Empty(t *testing.T) {
	idx, total, err := ParseShard("")
	if err != nil {
		t.Errorf("ParseShard empty: %v", err)
	}
	if idx != 0 || total != 0 {
		t.Errorf("ParseShard empty: got (%d, %d), want (0, 0)", idx, total)
	}
}

func TestMergeResults(t *testing.T) {
	po := &ParallelOrchestrator{
		devices: []DeviceRun{
			{DeviceID: "dev1", Results: []TestResult{{TestName: "t1", Passed: true}}},
			{DeviceID: "dev2", Results: []TestResult{{TestName: "t2", Passed: false}}},
			{DeviceID: "dev3", Results: []TestResult{{TestName: "t3", Passed: true}, {TestName: "t4", Passed: true}}},
		},
	}

	merged := po.mergeResults()
	if len(merged) != 4 {
		t.Fatalf("expected 4 results, got %d", len(merged))
	}
	if merged[0].TestName != "t1" || merged[1].TestName != "t2" || merged[2].TestName != "t3" {
		t.Error("merge order incorrect")
	}
}

func TestSummaryError_NoErrors(t *testing.T) {
	po := &ParallelOrchestrator{
		devices: []DeviceRun{
			{DeviceID: "dev1", Results: []TestResult{{Passed: true}}},
			{DeviceID: "dev2", Results: []TestResult{{Passed: true}}},
		},
	}
	if err := po.summaryError(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSummaryError_WithErrors(t *testing.T) {
	po := &ParallelOrchestrator{
		devices: []DeviceRun{
			{DeviceID: "dev1", Error: fmt.Errorf("connection refused")},
			{DeviceID: "dev2", Results: []TestResult{{Passed: true}}},
		},
	}
	if err := po.summaryError(); err == nil {
		t.Error("expected error for failed device")
	}
}
