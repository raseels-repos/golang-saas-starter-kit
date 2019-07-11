package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errExpectedFailure = errors.New("expected failure for test purposes")

func TestDelayer(t *testing.T) {
	delays := []time.Duration{
		time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		10 * time.Millisecond,
	}
	tt := []struct {
		desc       string
		numRetries int
		expDelay   time.Duration
	}{
		{"first try", 0, time.Millisecond},
		{"second try", 1, 2 * time.Millisecond},
		{"len(delays) try", len(delays) - 1, delays[len(delays)-1]},
		{"len(delays) + 1 try", len(delays), delays[len(delays)-1]},
		{"len(delays) * 2 try", len(delays) * 2, delays[len(delays)-1]},
	}

	for _, tc := range tt {
		t.Run(tc.desc, func(t *testing.T) {
			var (
				d     = delayer{Delays: delays}
				delay time.Duration
			)
			for i := tc.numRetries + 1; i > 0; i-- {
				delay = d.Delay()
			}
			if delay != tc.expDelay {
				t.Fatalf(
					"expected delay of %s after %d retries, but got %s",
					tc.expDelay, tc.numRetries, delay)
			}
		})

	}
}

func TestRetry(t *testing.T) {
	delays := []time.Duration{
		time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
	}
	tt := []struct {
		desc    string
		tries   int
		success bool
		err     error
	}{
		{"first try", 1, true, nil},
		{"second try error", 2, false, errExpectedFailure},
		{"third try success", 3, true, nil},
	}
	for _, tc := range tt {
		t.Run(tc.desc, func(t *testing.T) {
			tries := 0
			retryFunc := func() (bool, error) {
				tries++
				if tries == tc.tries {
					return tc.success, tc.err
				}
				t.Logf("try #%d unsuccessful: trying again up to %d times", tries, tc.tries)
				return false, nil
			}
			err := Retry(context.Background(), delays, retryFunc)
			if err != tc.err {
				t.Fatalf("expected error %s, but got error %s", err, tc.err)
			}
			if tries != tc.tries {
				t.Fatalf("expected %d tries, but tried %d times", tc.tries, tries)
			}
		})
	}
}
