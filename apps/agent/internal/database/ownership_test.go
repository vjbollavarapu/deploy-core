package database

import "testing"

func TestVerifyManagedDatabaseLabels(t *testing.T) {
	labels := map[string]string{
		"deploycore.managed":      "true",
		"deploycore.service_type": "database",
		"deploycore.database_id":  "abc-123",
	}
	if err := VerifyManagedDatabaseLabels(labels, "abc-123"); err != nil {
		t.Fatalf("expected match: %v", err)
	}
	if err := VerifyManagedDatabaseLabels(labels, "other"); err == nil {
		t.Fatal("expected mismatch error")
	}
	if err := VerifyManagedDatabaseLabels(map[string]string{"deploycore.managed": "true"}, "abc-123"); err == nil {
		t.Fatal("expected missing service_type error")
	}
}

func TestValidateImageVersion(t *testing.T) {
	if err := ValidateImageVersion("16"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImageVersion("evil/image:latest"); err == nil {
		t.Fatal("expected opaque image rejection")
	}
}
