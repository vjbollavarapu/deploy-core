package deployments

import "testing"

func TestCanTransitionHappyPath(t *testing.T) {
	t.Parallel()
	for i := 0; i < len(happyPath)-1; i++ {
		from, to := happyPath[i], happyPath[i+1]
		if !CanTransition(from, to) {
			t.Fatalf("expected %s → %s", from, to)
		}
	}
	if CanTransition(StatusRunning, StatusQueued) {
		t.Fatal("RUNNING should not transition to QUEUED")
	}
	if !IsTerminal(StatusRunning) || !IsTerminal(StatusCancelled) {
		t.Fatal("expected terminal statuses")
	}
	if err := ValidateTransition(StatusBuilding, StatusImageReady); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransition(StatusBuilding, StatusActivating); err == nil {
		t.Fatal("expected invalid transition")
	}
	if err := ValidateTransition(StatusCancelled, StatusQueued); err == nil {
		t.Fatal("expected terminal immutable")
	}
}

func TestCancelFromInProgress(t *testing.T) {
	t.Parallel()
	for _, from := range []string{StatusQueued, StatusBuilding, StatusActivating} {
		if !CanTransition(from, StatusCancelled) {
			t.Fatalf("cancel from %s", from)
		}
	}
}

func TestNextHappyPath(t *testing.T) {
	t.Parallel()
	next, ok := NextHappyPath(StatusPending)
	if !ok || next != StatusQueued {
		t.Fatalf("got %s %v", next, ok)
	}
	_, ok = NextHappyPath(StatusRunning)
	if ok {
		t.Fatal("no next after RUNNING")
	}
}

func TestFailureTerminalsFromStages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to string
	}{
		{StatusFetchingSource, StatusSourceFailed},
		{StatusBuilding, StatusBuildFailed},
		{StatusBuilding, StatusImageFailed},
		{StatusCreatingContainer, StatusContainerFailed},
		{StatusStarting, StatusStartFailed},
		{StatusHealthChecking, StatusHealthCheckFailed},
		{StatusActivating, StatusRoutingFailed},
	}
	for _, tc := range cases {
		if !CanTransition(tc.from, tc.to) {
			t.Fatalf("expected %s → %s", tc.from, tc.to)
		}
		if !IsFailureTerminal(tc.to) {
			t.Fatalf("%s should be failure terminal", tc.to)
		}
		if err := ValidateTransition(tc.to, StatusQueued); err == nil {
			t.Fatalf("terminal %s must not leave to QUEUED", tc.to)
		}
	}
}

func TestTimeoutAndCancelFromQueued(t *testing.T) {
	t.Parallel()
	if !CanTransition(StatusQueued, StatusTimeout) || !CanTransition(StatusQueued, StatusCancelled) {
		t.Fatal("queued must allow timeout and cancel")
	}
}
