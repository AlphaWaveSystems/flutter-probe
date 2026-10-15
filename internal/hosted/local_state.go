package hosted

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Local runs keep two small files in one directory the CLI passes in (~/.flutterprobe/local-upload):
//   signer      the installation's signer: 32 lower-case hex characters, created once. It is random, never derived from
//               the machine, the repository or the person.
//   state.json  the last sequence number reserved for that signer.
// The server's idempotency key is (signer, seq), so a sequence number must never be reused for a signer.

const (
	signerFile = "signer"
	stateFile  = "state.json"
	lockFile   = "state.lock"
	// lockStale is the age after which a lock file is taken to belong to a run that died.
	lockStale = 30 * time.Second
)

// lockWait bounds how long a reservation waits for another run; tests shorten it.
var lockWait = 5 * time.Second

// Signer returns the installation's signer, creating it on first use. A file that is not a valid signer is refused
// rather than replaced: a new signer starts a new sequence, and that should be the user's choice.
func Signer(dir string) (string, error) {
	path := filepath.Join(dir, signerFile)
	raw, err := os.ReadFile(path)
	if err == nil {
		return checkSigner(path, raw)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	random := make([]byte, 16) // 128 random bits
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random)
	// The file appears whole or not at all: it is written under a temporary name and then linked into place, which fails
	// when another run got there first. That run's signer is the one to use.
	tmp, err := os.CreateTemp(dir, "signer-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(id + "\n"); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Link(tmp.Name(), path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			return "", rerr
		}
		return checkSigner(path, raw)
	}
	return id, nil
}

func checkSigner(path string, raw []byte) (string, error) {
	id := strings.TrimSpace(string(raw))
	if !signerID.MatchString(id) {
		return "", fmt.Errorf("the local signer file %s is not valid; remove it to create a new signer (this starts a new sequence)", path)
	}
	return id, nil
}

// NextSeq reserves the next sequence number for signer and writes it down before it returns, so the number is never
// handed out twice, even when the upload that uses it fails or is retried. The number is the larger of the last one
// plus one and the current time in milliseconds. That keeps it strictly increasing when the state file is lost or the
// clock steps back, and it stays far below 2^53-1.
func NextSeq(dir, signer string, now time.Time) (int64, error) {
	return reserveSeq(dir, signer, now, true)
}

// PeekSeq returns the number NextSeq would reserve now, and writes nothing. A dry run uses it.
func PeekSeq(dir, signer string, now time.Time) (int64, error) {
	return reserveSeq(dir, signer, now, false)
}

// seqState is the content of the state file.
type seqState struct {
	Signer string `json:"signer"`
	Last   int64  `json:"last"`
}

func reserveSeq(dir, signer string, now time.Time, persist bool) (int64, error) {
	if !signerID.MatchString(signer) {
		return 0, errors.New("the signer is not valid")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	if persist {
		unlock, err := lockState(dir)
		if err != nil {
			return 0, err
		}
		defer unlock()
	}
	st := readSeqState(dir, signer)
	next := st.Last + 1
	if ms := now.UnixMilli(); ms > next {
		next = ms
	}
	if next > maxSeq {
		return 0, errors.New("the local sequence number is exhausted")
	}
	if persist {
		if err := writeSeqState(dir, seqState{Signer: signer, Last: next}); err != nil {
			return 0, err
		}
	}
	return next, nil
}

// readSeqState returns the stored state for signer. A missing file, a file for another signer and a file that cannot be
// parsed all read as the zero state. That is safe because the number is never below the current time in milliseconds,
// which is above every number an earlier run could have used.
func readSeqState(dir, signer string) seqState {
	raw, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return seqState{}
	}
	var st seqState
	if json.Unmarshal(raw, &st) != nil || st.Signer != signer || st.Last < 0 || st.Last > maxSeq {
		return seqState{}
	}
	return st
}

// writeSeqState replaces the state file atomically, so a crash leaves the old state or the new one, never a torn file.
func writeSeqState(dir string, st seqState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, stateFile))
}

// lockState takes the state lock: a file created exclusively, removed by the returned function. A lock older than
// lockStale belongs to a run that died and is taken over. A lock that stays busy for lockWait is an error, never a
// second writer.
func lockState(dir string) (func(), error) {
	path := filepath.Join(dir, lockFile)
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if fi, serr := os.Stat(path); serr == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(path)
		} else if time.Now().After(deadline) {
			return nil, errors.New("another probe run is reserving a local upload sequence number; try again")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
