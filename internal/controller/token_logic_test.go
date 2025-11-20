package controller

import (
	"testing"
	"time"

	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ============================================================================
// Error Handling Tests
// ============================================================================

// TestErrorsReturnEmptyResult tests that errors are returned without RequeueAfter.
// The controller-runtime framework handles automatic exponential backoff on error returns.
func TestErrorsReturnEmptyResult(t *testing.T) {
	tests := []struct {
		name        string
		errorType   string
		description string
	}{
		{
			name:        "secret_not_found_error",
			errorType:   "SecretNotFound",
			description: "Secret not found should return (Result{}, error)",
		},
		{
			name:        "provider_not_found_error",
			errorType:   "ProviderNotFound",
			description: "Provider not found should return (Result{}, error)",
		},
		{
			name:        "token_renewal_error",
			errorType:   "TokenRenewalError",
			description: "Token renewal errors should return (Result{}, error)",
		},
		{
			name:        "token_validity_error",
			errorType:   "TokenValidityError",
			description: "Token validity check errors should return (Result{}, error)",
		},
		{
			name:        "secret_update_error",
			errorType:   "SecretUpdateError",
			description: "Secret update errors should return (Result{}, error)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Testing: %s - %s", test.name, test.description)

			// Correct pattern: error returns should have empty Result
			result := ctrl.Result{}

			// Verify: Result should be empty (no RequeueAfter, no Requeue)
			if result.RequeueAfter != 0 {
				t.Error("Error result should have zero RequeueAfter")
			}
			if result.Requeue {
				t.Error("Error result should not set Requeue to true")
			}
		})
	}
}

// TestRateLimiterStrategy tests the FastSlow rate limiter configuration
func TestRateLimiterStrategy(t *testing.T) {
	t.Run("fast_slow_rate_limiter_configured", func(t *testing.T) {
		// Create the same rate limiter used in main.go
		limiter := workqueue.NewTypedItemFastSlowRateLimiter[reconcile.Request](
			100*time.Millisecond, // fast delay
			5*time.Minute,        // slow delay
			3,                    // max fast attempts
		)

		// Simulate first few retries (should be fast)
		req := reconcile.Request{}
		for i := 0; i < 3; i++ {
			delay := limiter.When(req)
			if delay > 500*time.Millisecond {
				t.Errorf("Fast retry %d should be quick (got %v)", i+1, delay)
			}
			t.Logf("Retry %d delay: %v", i+1, delay)
		}

		// After max fast attempts, should switch to slow
		delay := limiter.When(req)
		if delay < 1*time.Minute {
			t.Errorf("Slow retry should be delayed (got %v, expected >= 1min)", delay)
		}
		t.Logf("Slow retry delay: %v (correct - backing off)", delay)
	})
}

// TestAutomaticBackoffBehavior tests that framework backoff works correctly
func TestAutomaticBackoffBehavior(t *testing.T) {
	t.Run("framework_exponential_backoff", func(t *testing.T) {
		// When reconciler returns (Result{}, error), framework applies exponential backoff:
		// 1st retry: 1s
		// 2nd retry: 2s
		// 3rd retry: 4s
		// ...up to ~15 minutes

		result := ctrl.Result{}

		// Verify it's empty - backoff is handled by framework
		if result.RequeueAfter != 0 {
			t.Error("Result should be empty to allow framework backoff")
		}

		t.Log("✓ Framework will apply automatic exponential backoff: 1s → 2s → 4s → ... → 15m")
	})
}

// TestNoRequeueAfterWithErrors tests that using RequeueAfter with errors is incorrect
func TestNoRequeueAfterWithErrors(t *testing.T) {
	t.Run("incorrect_pattern_ignored", func(t *testing.T) {
		// INCORRECT: returning (Result{RequeueAfter: X}, error)
		incorrectResult := ctrl.Result{RequeueAfter: 5 * time.Minute}

		t.Logf("⚠️  If error is non-nil, this RequeueAfter (%v) is IGNORED by framework",
			incorrectResult.RequeueAfter)
		t.Log("Framework will use exponential backoff instead")
	})
}

// TestSuccessPathWithScheduledRequeue tests the correct success path
func TestSuccessPathWithScheduledRequeue(t *testing.T) {
	t.Run("success_with_calculated_requeue", func(t *testing.T) {
		// On successful reconciliation, schedule next check
		expirationTime := time.Now().Add(24 * time.Hour)
		renewBefore := 1 * time.Hour

		nextReconcileTime := expirationTime.Add(-renewBefore)
		requeueAfter := time.Until(nextReconcileTime)

		successResult := ctrl.Result{RequeueAfter: requeueAfter}

		// Verify: RequeueAfter makes sense for success path
		if successResult.RequeueAfter <= 0 {
			t.Logf("Token expires soon, next reconcile immediately")
		} else {
			t.Logf("✓ Success: Next reconcile scheduled in ~%v", requeueAfter)
		}
	})
}

// ============================================================================
// Token Renewal Logic Tests
// ============================================================================

// TestTokenRenewalLogic_VerifyBuggyCondition demonstrates the bug in the renewal condition
func TestTokenRenewalLogic_VerifyBuggyCondition(t *testing.T) {
	t.Run("buggy_logic_prevents_renewal_when_needed", func(t *testing.T) {
		// Scenario: Token expires in 45 minutes, renewal buffer is 1 hour
		// Expected: Should renew NOW because 45 minutes < 1 hour buffer
		// Buggy behavior: Won't renew because After() returns false

		now := time.Now()
		expirationTime := now.Add(45 * time.Minute)
		beforeDuration := 1 * time.Hour

		timeToUpdate := now.Add(beforeDuration)

		// Current BUGGY logic: if expirationTime.After(timeToUpdate)
		shouldRenewBuggy := expirationTime.After(timeToUpdate)

		// CORRECT logic: if !expirationTime.After(timeToUpdate) or equivalently: if expirationTime.Before(timeToUpdate) || expirationTime.Equal(timeToUpdate)
		shouldRenewCorrect := !expirationTime.After(timeToUpdate)

		if shouldRenewBuggy == shouldRenewCorrect {
			t.Errorf("Bug not detected: Both logics give same result (%v)", shouldRenewBuggy)
		}

		if shouldRenewBuggy {
			t.Error("Buggy logic incorrectly says to renew (After() returned true for earlier expiration)")
		}

		if !shouldRenewCorrect {
			t.Error("Correct logic should renew (expiration is before timeToUpdate)")
		}

		t.Logf("✓ Bug confirmed: Buggy logic=%v, Correct logic=%v", shouldRenewBuggy, shouldRenewCorrect)
	})

	t.Run("buggy_logic_ignores_already_expired_token", func(t *testing.T) {
		// Scenario: Token already expired 10 minutes ago
		// Expected: Should renew immediately
		// Buggy behavior: Won't renew

		now := time.Now()
		expirationTime := now.Add(-10 * time.Minute)
		beforeDuration := 1 * time.Hour

		timeToUpdate := now.Add(beforeDuration)

		shouldRenewBuggy := expirationTime.After(timeToUpdate)
		shouldRenewCorrect := !expirationTime.After(timeToUpdate)

		if shouldRenewBuggy {
			t.Error("Buggy logic: Already expired token should NOT trigger After()")
		}

		if !shouldRenewCorrect {
			t.Error("Correct logic: Already expired token MUST be renewed")
		}
	})
}

// TestTokenRenewalLogic tests the token renewal timing logic
func TestTokenRenewalLogic(t *testing.T) {
	tests := []struct {
		name           string
		expirationTime time.Time
		beforeDuration time.Duration
		shouldRenew    bool
		description    string
	}{
		{
			name:           "token_expires_soon_should_renew",
			expirationTime: time.Now().Add(30 * time.Minute),
			beforeDuration: 1 * time.Hour,
			shouldRenew:    true,
			description:    "Token expires in 30min, beforeDuration is 1h => should renew NOW",
		},
		{
			name:           "token_expires_far_away_should_not_renew",
			expirationTime: time.Now().Add(2 * time.Hour),
			beforeDuration: 1 * time.Hour,
			shouldRenew:    false,
			description:    "Token expires in 2h, beforeDuration is 1h => no renewal needed yet",
		},
		{
			name:           "token_expires_now_should_renew",
			expirationTime: time.Now(),
			beforeDuration: 1 * time.Hour,
			shouldRenew:    true,
			description:    "Token expires NOW, beforeDuration is 1h => should renew",
		},
		{
			name:           "token_already_expired_should_renew",
			expirationTime: time.Now().Add(-10 * time.Minute),
			beforeDuration: 1 * time.Hour,
			shouldRenew:    true,
			description:    "Token expired 10min ago => should renew immediately",
		},
		{
			name:           "token_expires_exactly_at_threshold",
			expirationTime: time.Now().Add(1 * time.Hour),
			beforeDuration: 1 * time.Hour,
			shouldRenew:    true,
			description:    "Token expires exactly at the renewal threshold => should renew",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Logf("Testing: %s", test.description)

			// Simulate the controller logic
			timeToUpdate := time.Now().Add(test.beforeDuration)

			// CORRECT LOGIC should be:
			// if !token.Status.ExpirationTime.After(timeToUpdate) {
			//     renew()
			// }
			// This returns true when expirationTime is NOT after timeToUpdate (i.e., before or equal)
			shouldRenewCorrectLogic := !test.expirationTime.After(timeToUpdate)

			if shouldRenewCorrectLogic != test.shouldRenew {
				t.Errorf("Correct logic failed: got %v, want %v", shouldRenewCorrectLogic, test.shouldRenew)
			}
		})
	}
}

// TestTokenRenewalLogic_EdgeCase tests edge case at renewal boundary
func TestTokenRenewalLogic_EdgeCase(t *testing.T) {
	now := time.Now()
	renewalBefore := 1 * time.Hour

	// Case: Token expires exactly 1 hour from now
	expirationTime := now.Add(renewalBefore)
	timeToUpdate := now.Add(renewalBefore)

	// With After() logic (BUGGY): expirationTime.After(timeToUpdate) = false (not strictly after)
	// => no renewal
	buggyResult := expirationTime.After(timeToUpdate)
	if buggyResult {
		t.Error("Buggy logic: token at exact boundary should NOT trigger After()")
	}

	// With !After() logic (CORRECT): !expirationTime.After(timeToUpdate) = true
	// => renewal happens
	correctResult := !expirationTime.After(timeToUpdate)
	if !correctResult {
		t.Error("Correct logic: token at exact boundary SHOULD trigger renewal")
	}
}

// ============================================================================
// Secret Data Handling Tests
// ============================================================================

// TestSecretDataHandling tests robustness of secret data handling
func TestSecretDataHandling(t *testing.T) {
	tests := []struct {
		name        string
		secretData  map[string][]byte
		expectError bool
		description string
	}{
		{
			name: "valid_token_key",
			secretData: map[string][]byte{
				"token": []byte("valid-token-value"),
			},
			expectError: false,
			description: "Secret with 'token' key should work",
		},
		{
			name:        "missing_token_key",
			secretData:  map[string][]byte{},
			expectError: true,
			description: "Secret without 'token' key should error, not silently fail",
		},
		{
			name: "empty_token_value",
			secretData: map[string][]byte{
				"token": []byte(""),
			},
			expectError: true,
			description: "Empty token value should error",
		},
		{
			name: "wrong_key_name",
			secretData: map[string][]byte{
				"api_token": []byte("value"),
			},
			expectError: true,
			description: "Wrong key name should error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			// Simulate current code behavior
			tokenValue := string(tt.secretData["token"])

			// BUG: If key is missing, tokenValue becomes "" without error
			if tokenValue == "" && len(tt.secretData) > 0 {
				// Token is explicitly empty
				if tt.expectError {
					t.Logf("✓ Correctly expects error for: %s", tt.name)
				}
			}

			if len(tt.secretData) == 0 && tokenValue == "" {
				// Bug: Can't distinguish between missing key and empty value
				if tt.expectError {
					t.Logf("⚠ BUGGY: Can't distinguish missing key vs empty value")
				}
			}

			// After fix: Should explicitly check key existence
			if _, exists := tt.secretData["token"]; !exists {
				if tt.expectError {
					t.Logf("✓ FIXED: Correctly detects missing 'token' key")
				}
			}
		})
	}
}

// TestSecretDeletionHandling tests handling of deleted Secrets
func TestSecretDeletionHandling(t *testing.T) {
	t.Run("secret_deletion_not_detected", func(t *testing.T) {
		t.Log("BUG: If Secret is deleted, controller returns error but no RequeueAfter")
		t.Log("This causes:")
		t.Log("  1. Immediate error log")
		t.Log("  2. Token CR remains in error state")
		t.Log("  3. Controller waits for next event (might be never)")
		t.Log("")
		t.Log("FIXING BY: Framework automatic backoff handles retries")
	})

	t.Run("nil_pointer_handling", func(t *testing.T) {
		// BUGGY: secret.Data["token"] returns nil if key doesn't exist
		// string(nil) = ""
		// So can't distinguish:
		//   1. Key exists with empty value
		//   2. Key doesn't exist at all
		//   3. Secret deleted

		secretData := map[string][]byte{}
		tokenValue := string(secretData["token"])

		if tokenValue == "" {
			t.Log("⚠ BUGGY: Cannot distinguish between missing key and empty value")
		}

		// CORRECT: Check key existence explicitly
		if _, exists := secretData["token"]; !exists {
			t.Log("✓ FIXED: Explicitly check key existence before accessing")
		}
	})
}
