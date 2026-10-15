package hosted

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSignerIsCreatedOnceAndKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local-upload")
	first, err := Signer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !signerID.MatchString(first) {
		t.Fatalf("signer = %q, want 32 lower-case hex characters", first)
	}
	second, err := Signer(dir)
	if err != nil || second != first {
		t.Fatalf("second call = %q, %v; want the same signer %q", second, err, first)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, signerFile))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("signer file mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestSignerRefusesACorruptFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, signerFile, "not-a-signer\n")
	if _, err := Signer(dir); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("err = %v, want a refusal that names the file", err)
	}
}

func TestNextSeqIncreasesAndIsWrittenBeforeItReturns(t *testing.T) {
	dir := t.TempDir()
	signer := strings.Repeat("d", 32)
	t0 := time.UnixMilli(1_800_000_000_000)

	s1, err := NextSeq(dir, signer, t0)
	if err != nil || s1 != t0.UnixMilli() {
		t.Fatalf("first = %d, %v; want the clock in milliseconds %d", s1, err, t0.UnixMilli())
	}
	s2, err := NextSeq(dir, signer, t0) // the same instant: last plus one
	if err != nil || s2 != s1+1 {
		t.Fatalf("second = %d, %v; want %d", s2, err, s1+1)
	}
	s3, err := NextSeq(dir, signer, t0.Add(-time.Hour)) // the clock stepped back: still increasing
	if err != nil || s3 != s2+1 {
		t.Fatalf("after a clock step back = %d, %v; want %d", s3, err, s2+1)
	}
	if st := readSeqState(dir, signer); st.Last != s3 {
		t.Fatalf("stored last = %d, want %d: the number must be on disk before it is used", st.Last, s3)
	}
}

func TestNextSeqSurvivesALostStateFile(t *testing.T) {
	dir := t.TempDir()
	signer := strings.Repeat("e", 32)
	t0 := time.UnixMilli(1_800_000_000_000)
	last, err := NextSeq(dir, signer, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, stateFile)); err != nil {
		t.Fatal(err)
	}
	// Losing the state cannot reuse a number: the clock is above every number an earlier run could have used.
	again, err := NextSeq(dir, signer, t0.Add(time.Second))
	if err != nil || again <= last {
		t.Fatalf("after the state was lost = %d, %v; want a number above %d", again, err, last)
	}
}

func TestPeekSeqWritesNothing(t *testing.T) {
	dir := t.TempDir()
	signer := strings.Repeat("f", 32)
	t0 := time.UnixMilli(1_800_000_000_000)
	peeked, err := PeekSeq(dir, signer, t0)
	if err != nil || peeked != t0.UnixMilli() {
		t.Fatalf("peek = %d, %v", peeked, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stateFile)); !os.IsNotExist(err) {
		t.Fatal("a peek wrote the state file")
	}
	reserved, err := NextSeq(dir, signer, t0)
	if err != nil || reserved != peeked {
		t.Fatalf("reserve = %d, %v; want the peeked number %d", reserved, err, peeked)
	}
}

func TestStateLockBusyThenStale(t *testing.T) {
	dir := t.TempDir()
	signer := strings.Repeat("a", 32)
	t0 := time.UnixMilli(1_800_000_000_000)
	defer func(d time.Duration) { lockWait = d }(lockWait)
	lockWait = 100 * time.Millisecond

	lock := filepath.Join(dir, lockFile)
	writeTestFile(t, dir, lockFile, "")
	if _, err := NextSeq(dir, signer, t0); err == nil || !strings.Contains(err.Error(), "another probe run") {
		t.Fatalf("a busy lock: err = %v, want a refusal to write a second reservation", err)
	}

	// A lock left behind by a run that died is taken over.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := NextSeq(dir, signer, t0); err != nil {
		t.Fatalf("a stale lock: %v", err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("the lock was not released")
	}
}
