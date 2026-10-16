package record

import (
	"context"
	"sort"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// RingSize is the number of exchanges retained by the agent.
const RingSize = 300

// LogSource supplies the agent's finished HTTP exchanges.
type LogSource interface {
	HTTPLog(context.Context, probelink.HTTPLogParams) (probelink.HTTPLogResult, error)
}

// Cursor tracks handled sequences without skipping requests still in flight.
// A Cursor must not be used concurrently.
type Cursor struct {
	epoch   string
	started bool
	contig  int
	seen    map[int]bool
}

// Read contains new exchanges and discontinuities observed during a poll.
type Read struct {
	Exchanges []probelink.HTTPEntry
	Restarted bool
	Lost      int
}

func NewCursor() *Cursor { return &Cursor{seen: make(map[int]bool)} }

// Next polls from the last contiguous handled sequence. Failed polls, including
// a failed restart refetch, leave the cursor unchanged.
func (c *Cursor) Next(ctx context.Context, src LogSource) (Read, error) {
	res, err := src.HTTPLog(ctx, probelink.HTTPLogParams{Since: c.contig})
	if err != nil {
		return Read{}, err
	}
	var read Read
	if c.started && res.Epoch != c.epoch {
		res, err = src.HTTPLog(ctx, probelink.HTTPLogParams{Since: 0})
		if err != nil {
			return Read{}, err
		}
		c.contig = 0
		c.seen = make(map[int]bool)
		read.Restarted = true
	}
	c.epoch, c.started = res.Epoch, true
	if c.seen == nil {
		c.seen = make(map[int]bool)
	}
	for _, entry := range res.Entries {
		if entry.Seq > c.contig && !c.seen[entry.Seq] {
			c.seen[entry.Seq] = true
			read.Exchanges = append(read.Exchanges, entry)
		}
	}
	sort.Slice(read.Exchanges, func(i, j int) bool {
		return read.Exchanges[i].Seq < read.Exchanges[j].Seq
	})
	// Advance over evicted sequences, counting only those never delivered.
	if floor := res.Latest - RingSize; floor > c.contig {
		read.Lost = floor - c.contig
		for seq := range c.seen {
			if seq <= floor {
				read.Lost--
				delete(c.seen, seq)
			}
		}
		c.contig = floor
	}
	for c.seen[c.contig+1] {
		delete(c.seen, c.contig+1)
		c.contig++
	}
	return read, nil
}
