package process

import (
	"errors"
	"testing"
	"time"
)

func TestDoWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	config := RetryConfig{
		MaxRetries: 3,
		MinDelay:   1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
	}

	err := DoWithRetry(fn, config)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoWithRetry_SuccessAfterRetry(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("temporary error")
		}
		return nil
	}

	config := RetryConfig{
		MaxRetries: 5,
		MinDelay:   1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
	}

	err := DoWithRetry(fn, config)
	if err != nil {
		t.Errorf("Expected no error after retry, got: %v", err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestDoWithRetry_AllRetriesExhausted(t *testing.T) {
	expectedErr := errors.New("persistent error")
	callCount := 0
	fn := func() error {
		callCount++
		return expectedErr
	}

	config := RetryConfig{
		MaxRetries: 3,
		MinDelay:   1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
	}

	err := DoWithRetry(fn, config)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
}

func TestDoWithRetry_MaxRetriesOne(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("error")
	}

	config := RetryConfig{
		MaxRetries: 1,
		MinDelay:   1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
	}

	err := DoWithRetry(fn, config)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoWithRetryDefault(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	err := DoWithRetryDefault(fn)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoWithRetryDefault_Exhausted(t *testing.T) {
	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("error")
	}

	err := DoWithRetryDefault(fn)
	if err == nil {
		t.Error("Expected error, got nil")
	}
	// DefaultRetryConfig has MaxRetries: 2
	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}
}

func TestDoWithRetry_ExponentialBackoff(t *testing.T) {
	callCount := 0
	var delays []time.Duration
	start := time.Now()

	fn := func() error {
		callCount++
		delays = append(delays, time.Since(start))
		return errors.New("error")
	}

	config := RetryConfig{
		MaxRetries: 3,
		MinDelay:   10 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	_ = DoWithRetry(fn, config)

	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}

	// First call should be immediate (no delay before first attempt)
	// Second call should have at least MinDelay (10ms) delay
	if len(delays) >= 2 {
		delay1 := delays[1] - delays[0]
		if delay1 < 10*time.Millisecond {
			t.Errorf("Expected second attempt delay >= 10ms, got %v", delay1)
		}
	}

	// Third call should have at least 2*MinDelay (20ms) delay from second
	if len(delays) >= 3 {
		delay2 := delays[2] - delays[1]
		if delay2 < 20*time.Millisecond {
			t.Errorf("Expected third attempt delay >= 20ms, got %v", delay2)
		}
	}
}

func TestDoWithRetry_MaxDelayCap(t *testing.T) {
	fn := func() error {
		return errors.New("error")
	}

	// MinDelay * 2^attempt could exceed MaxDelay, should be capped
	config := RetryConfig{
		MaxRetries: 5,
		MinDelay:   50 * time.Millisecond,
		MaxDelay:   100 * time.Millisecond,
	}

	start := time.Now()
	_ = DoWithRetry(fn, config)
	elapsed := time.Since(start)

	// With 5 attempts, delays are: 0, 50ms, 100ms(capped), 100ms(capped), 100ms(capped)
	// Plus jitter of up to 2s per retry (4 retries with delay = up to 8s jitter)
	// Allow generous margin: 12s
	if elapsed > 12*time.Second {
		t.Errorf("Expected total time < 12s with capped delays, got %v", elapsed)
	}
}

func TestRetryConfig_Defaults(t *testing.T) {
	if DefaultRetryConfig.MaxRetries != 2 {
		t.Errorf("Expected MaxRetries=2, got %d", DefaultRetryConfig.MaxRetries)
	}
	if DefaultRetryConfig.MinDelay != 5*time.Second {
		t.Errorf("Expected MinDelay=5s, got %v", DefaultRetryConfig.MinDelay)
	}
	if DefaultRetryConfig.MaxDelay != 20*time.Second {
		t.Errorf("Expected MaxDelay=20s, got %v", DefaultRetryConfig.MaxDelay)
	}
}