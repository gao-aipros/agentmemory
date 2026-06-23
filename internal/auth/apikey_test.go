package auth

import (
	"testing"
)

func TestSafeSlice_SufficientLength_ReturnsSlice(t *testing.T) {
	result, err := SafeSlice("abcdefgh", 8)
	if err != nil {
		t.Fatalf("expected no error for sufficient length, got: %v", err)
	}
	if result != "abcdefgh" {
		t.Fatalf("expected 'abcdefgh', got '%s'", result)
	}
}

func TestSafeSlice_ExactLength_ReturnsSlice(t *testing.T) {
	result, err := SafeSlice("12345678", 8)
	if err != nil {
		t.Fatalf("expected no error for exact length, got: %v", err)
	}
	if result != "12345678" {
		t.Fatalf("expected '12345678', got '%s'", result)
	}
}

func TestSafeSlice_ShortString_ReturnsError(t *testing.T) {
	_, err := SafeSlice("short", 8)
	if err == nil {
		t.Fatal("expected error for short string, got nil")
	}
}

func TestSafeSlice_EmptyString_ReturnsError(t *testing.T) {
	_, err := SafeSlice("", 8)
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
}

func TestSafeSlice_ZeroLength_ReturnsEmpty(t *testing.T) {
	result, err := SafeSlice("hello", 0)
	if err != nil {
		t.Fatalf("expected no error for zero length, got: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty string, got '%s'", result)
	}
}
