package process

import (
	"math/rand"
	"time"
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxRetries int
	MinDelay   time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig provides sensible defaults for retry behavior
var DefaultRetryConfig = RetryConfig{
	MaxRetries: 2,
	MinDelay:   5 * time.Second,
	MaxDelay:   20 * time.Second,
}

// RetryFunc is a function that returns an error and can be retried
type RetryFunc func() error

// DoWithRetry executes a function with exponential backoff retry logic.
// It will retry up to maxRetries times with random delay between minDelay and maxDelay.
// Returns the last error if all retries fail, or nil on success.
func DoWithRetry(fn RetryFunc, config RetryConfig) error {
	var lastErr error
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		// Calculate random delay with exponential backoff
		baseDelay := config.MinDelay
		if attempt > 0 {
			// Exponential backoff: double the delay for each subsequent attempt
			baseDelay = config.MinDelay * time.Duration(1<<uint(attempt))
			if baseDelay > config.MaxDelay {
				baseDelay = config.MaxDelay
			}
		}

		// Add random jitter (up to 2 seconds)
		jitter := time.Duration(r.Intn(2000)) * time.Millisecond
		delay := baseDelay + jitter

		time.Sleep(delay)
	}

	return lastErr
}

// DoWithRetryDefault uses default retry configuration
func DoWithRetryDefault(fn RetryFunc) error {
	return DoWithRetry(fn, DefaultRetryConfig)
}