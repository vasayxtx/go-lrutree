package lrutree

import (
	"errors"
	"reflect"
	"testing"
)

func assertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("Not equal: \nexpected: %v\nactual  : %v\n", expected, actual)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Received unexpected error: %v\n", err)
	}
}

func assertErrorIs(t *testing.T, err, expectedErr error) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error: %v, got nil\n", expectedErr)
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Expected error: %v, got: %v\n", expectedErr, err)
	}
}

func assertTrue(t *testing.T, value bool) {
	t.Helper()
	if !value {
		t.Fatalf("Expected true but got false\n")
	}
}

func assertFalse(t *testing.T, value bool) {
	t.Helper()
	if value {
		t.Fatalf("Expected false but got true\n")
	}
}

func assertNil(t *testing.T, value interface{}) {
	t.Helper()
	if value != nil && !reflect.ValueOf(value).IsNil() {
		t.Fatalf("Expected nil, got: %v\n", value)
	}
}
