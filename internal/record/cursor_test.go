package record

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

type cursorReply struct {
	since  int
	result probelink.HTTPLogResult
	err    error
}

type cursorSource struct {
	t       *testing.T
	replies []cursorReply
	calls   int
}

func (s *cursorSource) HTTPLog(ctx context.Context, p probelink.HTTPLogParams) (probelink.HTTPLogResult, error) {
	s.t.Helper()
	if ctx == nil {
		s.t.Fatal("nil context")
	}
	if s.calls >= len(s.replies) {
		s.t.Fatal("unexpected HTTPLog call")
	}
	r := s.replies[s.calls]
	s.calls++
	if want := (probelink.HTTPLogParams{Since: r.since}); p != want {
		s.t.Fatalf("params = %+v, want %+v", p, want)
	}
	return r.result, r.err
}

func cursorResult(epoch string, latest int, seqs ...int) probelink.HTTPLogResult {
	r := probelink.HTTPLogResult{Epoch: epoch, Latest: latest}
	for _, seq := range seqs {
		r.Entries = append(r.Entries, probelink.HTTPEntry{Seq: seq})
	}
	return r
}

func TestCursor(t *testing.T) {
	boom := errors.New("poll failed")
	type step struct {
		replies   []cursorReply
		seqs      []int
		contig    int
		seen      []int
		lost      int
		restarted bool
		err       error
	}
	tests := []struct {
		name    string
		initial *Cursor
		steps   []step
	}{
		{name: "contiguous reads", steps: []step{
			{replies: []cursorReply{{0, cursorResult("a", 2, 2, 1), nil}}, seqs: []int{1, 2}, contig: 2},
			{replies: []cursorReply{{2, cursorResult("a", 3, 3), nil}}, seqs: []int{3}, contig: 3},
		}},
		{name: "in flight", steps: []step{
			{replies: []cursorReply{{0, cursorResult("a", 6, 6, 4, 3, 2, 1), nil}}, seqs: []int{1, 2, 3, 4, 6}, contig: 4, seen: []int{6}},
			{replies: []cursorReply{{4, cursorResult("a", 6, 6, 5), nil}}, seqs: []int{5}, contig: 6},
		}},
		{name: "overflow", initial: &Cursor{epoch: "a", started: true, contig: 10}, steps: []step{
			{replies: []cursorReply{{10, cursorResult("a", 400, 101), nil}}, seqs: []int{101}, contig: 101, lost: 90},
			{replies: []cursorReply{{101, cursorResult("a", 400, 101), nil}}, contig: 101},
		}},
		{name: "overflow excludes previously seen", initial: &Cursor{epoch: "a", started: true, contig: 10, seen: map[int]bool{50: true, 102: true}}, steps: []step{
			{replies: []cursorReply{{10, cursorResult("a", 400, 101, 102), nil}}, seqs: []int{101}, contig: 102, lost: 89},
		}},
		{name: "restart", initial: &Cursor{epoch: "a", started: true, contig: 10, seen: map[int]bool{12: true}}, steps: []step{
			{replies: []cursorReply{{10, cursorResult("b", 2), nil}, {0, cursorResult("b", 2, 2, 1), nil}}, seqs: []int{1, 2}, contig: 2, restarted: true},
			{replies: []cursorReply{{2, cursorResult("b", 3, 3), nil}}, seqs: []int{3}, contig: 3},
		}},
		{name: "duplicates", steps: []step{
			{replies: []cursorReply{{0, cursorResult("a", 2, 2, 2), nil}}, seqs: []int{2}, seen: []int{2}},
			{replies: []cursorReply{{0, cursorResult("a", 2, 2), nil}}, seen: []int{2}},
			{replies: []cursorReply{{0, cursorResult("a", 2, 2, 1, 1), nil}}, seqs: []int{1}, contig: 2},
		}},
		{name: "initial error", steps: []step{{replies: []cursorReply{{0, probelink.HTTPLogResult{}, boom}}, err: boom}}},
		{name: "error preserves pending", initial: &Cursor{epoch: "a", started: true, contig: 4, seen: map[int]bool{6: true}}, steps: []step{
			{replies: []cursorReply{{4, probelink.HTTPLogResult{}, boom}}, err: boom},
		}},
		{name: "restart refetch error", initial: &Cursor{epoch: "a", started: true, contig: 4, seen: map[int]bool{6: true}}, steps: []step{
			{replies: []cursorReply{{4, cursorResult("b", 2), nil}, {0, probelink.HTTPLogResult{}, boom}}, err: boom},
			{replies: []cursorReply{{4, cursorResult("b", 1), nil}, {0, cursorResult("b", 1, 1), nil}}, seqs: []int{1}, contig: 1, restarted: true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.initial
			if c == nil {
				c = NewCursor()
			}
			for i, st := range tt.steps {
				before := *c
				if c.seen != nil {
					before.seen = map[int]bool{}
					for seq, v := range c.seen {
						before.seen[seq] = v
					}
				}
				src := &cursorSource{t: t, replies: st.replies}
				got, err := c.Next(context.Background(), src)
				if !errors.Is(err, st.err) {
					t.Fatalf("step %d: error = %v, want %v", i, err, st.err)
				}
				if src.calls != len(src.replies) {
					t.Fatalf("step %d: calls = %d, want %d", i, src.calls, len(src.replies))
				}
				if st.err != nil {
					if !reflect.DeepEqual(*c, before) {
						t.Fatalf("step %d: cursor changed on error: %+v -> %+v", i, before, *c)
					}
					if !reflect.DeepEqual(got, Read{}) {
						t.Fatalf("read on error = %+v", got)
					}
					continue
				}
				var seqs []int
				for _, e := range got.Exchanges {
					seqs = append(seqs, e.Seq)
				}
				if !reflect.DeepEqual(seqs, st.seqs) || got.Lost != st.lost || got.Restarted != st.restarted {
					t.Fatalf("step %d: read = %+v, want seqs %v, lost %d, restarted %v", i, got, st.seqs, st.lost, st.restarted)
				}
				if c.contig != st.contig {
					t.Fatalf("step %d: contig = %d, want %d", i, c.contig, st.contig)
				}
				wantSeen := map[int]bool{}
				for _, seq := range st.seen {
					wantSeen[seq] = true
				}
				if !reflect.DeepEqual(c.seen, wantSeen) {
					t.Fatalf("step %d: seen = %v, want %v", i, c.seen, wantSeen)
				}
				if !c.started || c.epoch != st.replies[len(st.replies)-1].result.Epoch {
					t.Fatalf("step %d: epoch/started = %q/%v", i, c.epoch, c.started)
				}
			}
		})
	}
}
