package safety

import (
	"context"
	"errors"
	"testing"
)

type mockSystemResourceChecker struct {
	freeDisk  int64
	totalDisk int64
	diskErr   error

	availMem int64
	totalMem int64
	memErr   error

	dockerErr error
}

func (m *mockSystemResourceChecker) CheckDiskSpace(ctx context.Context, path string) (int64, int64, error) {
	return m.freeDisk, m.totalDisk, m.diskErr
}

func (m *mockSystemResourceChecker) CheckMemory(ctx context.Context) (int64, int64, error) {
	return m.availMem, m.totalMem, m.memErr
}

func (m *mockSystemResourceChecker) CheckDockerHealth(ctx context.Context) error {
	return m.dockerErr
}

func TestChecker_DockerUnavailable(t *testing.T) {
	mock := &mockSystemResourceChecker{
		dockerErr: errors.New("daemon connection refused"),
	}

	checker := NewChecker(mock, DefaultThresholds(), nil)
	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var sErr *SafetyError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *SafetyError, got %T", err)
	}
	if sErr.Code != CodeDockerUnavailable {
		t.Errorf("expected code %s, got %s", CodeDockerUnavailable, sErr.Code)
	}
}

func TestChecker_InsufficientDisk_Bytes(t *testing.T) {
	mock := &mockSystemResourceChecker{
		freeDisk:  500 * 1024 * 1024,        // 500 MB (below default 2GB)
		totalDisk: 100 * 1024 * 1024 * 1024, // 100 GB
		availMem:  1024 * 1024 * 1024,       // 1 GB
		totalMem:  8 * 1024 * 1024 * 1024,
	}

	checker := NewChecker(mock, DefaultThresholds(), nil)
	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var sErr *SafetyError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *SafetyError, got %T", err)
	}
	if sErr.Code != CodeInsufficientDisk {
		t.Errorf("expected code %s, got %s", CodeInsufficientDisk, sErr.Code)
	}
}

func TestChecker_InsufficientDisk_Percent(t *testing.T) {
	mock := &mockSystemResourceChecker{
		freeDisk:  3 * 1024 * 1024 * 1024,   // 3 GB (> 2GB min)
		totalDisk: 100 * 1024 * 1024 * 1024, // 3% free (< 5% min)
		availMem:  1024 * 1024 * 1024,
		totalMem:  8 * 1024 * 1024 * 1024,
	}

	checker := NewChecker(mock, DefaultThresholds(), nil)
	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var sErr *SafetyError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *SafetyError, got %T", err)
	}
	if sErr.Code != CodeInsufficientDisk {
		t.Errorf("expected code %s, got %s", CodeInsufficientDisk, sErr.Code)
	}
}

func TestChecker_InsufficientMemory(t *testing.T) {
	mock := &mockSystemResourceChecker{
		freeDisk:  50 * 1024 * 1024 * 1024,
		totalDisk: 100 * 1024 * 1024 * 1024,
		availMem:  100 * 1024 * 1024, // 100 MB (< 256 MB)
		totalMem:  8 * 1024 * 1024 * 1024,
	}

	checker := NewChecker(mock, DefaultThresholds(), nil)
	err := checker.ValidateExpensiveOperation(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var sErr *SafetyError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected *SafetyError, got %T", err)
	}
	if sErr.Code != CodeInsufficientMemory {
		t.Errorf("expected code %s, got %s", CodeInsufficientMemory, sErr.Code)
	}
}

func TestChecker_Success(t *testing.T) {
	mock := &mockSystemResourceChecker{
		freeDisk:  50 * 1024 * 1024 * 1024,
		totalDisk: 100 * 1024 * 1024 * 1024,
		availMem:  2 * 1024 * 1024 * 1024,
		totalMem:  8 * 1024 * 1024 * 1024,
	}

	checker := NewChecker(mock, DefaultThresholds(), nil)
	err := checker.ValidateExpensiveOperation(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
