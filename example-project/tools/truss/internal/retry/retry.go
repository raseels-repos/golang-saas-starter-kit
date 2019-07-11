// Package retry contains a simple retry mechanism defined by a slice of delay
// times. There are no maximum retries accounted for here. If retries should be
// limited, use a Timeout context to keep from retrying forever. This should
// probably be made into something more robust.
package retry

import (
	"context"
	"time"
)

// queryPollIntervals is a slice of the delays before re-checking the status on
// an executing query, backing off from a short delay at first. This sequence
// has been selected with Athena queries in mind, which may operate very
// quickly for things like schema manipulation, or which may run for an
// extended period of time, when running an actual data analysis query.
// Long-running queries will exhaust their rapid retries quickly, and fall back
// to checking every few seconds or longer.
var DefaultPollIntervals = []time.Duration{
	time.Millisecond,
	2 * time.Millisecond,
	2 * time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	20 * time.Millisecond,
	50 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	20 * time.Second,
	30 * time.Second,
	time.Minute,
}

// delayer keeps track of the current delay between retries.
type delayer struct {
	Delays       []time.Duration
	currentIndex int
}

// Delay returns the current delay duration, and advances the index to the next
// delay defined. If the index has reached the end of the delay slice, then it
// will continue to return the maximum delay defined.
func (d *delayer) Delay() time.Duration {
	t := d.Delays[d.currentIndex]
	if d.currentIndex < len(d.Delays)-1 {
		d.currentIndex++
	}
	return t
}

// Retry uses a slice of time.Duration interval delays to retry a function
// until it either errors or indicates that it is ready to proceed. If f
// returns true, or an error, the retry loop is broken. Pass a closure as f if
// you need to record a value from the operation that you are performing inside
// f.
func Retry(ctx context.Context, retryIntervals []time.Duration, f func() (bool, error)) (err error) {
	if retryIntervals == nil || len(retryIntervals) == 0 {
		retryIntervals = DefaultPollIntervals
	}

	d := delayer{Delays: retryIntervals}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			ok, err := f()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			time.Sleep(d.Delay())
		}
	}
	return err
}
