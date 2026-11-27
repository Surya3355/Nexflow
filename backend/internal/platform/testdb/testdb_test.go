package testdb

import (
	"context"
	"testing"
)

func TestNewReturnsWorkingPool(t *testing.T) {
	pool := New(t)

	var one int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}
}
