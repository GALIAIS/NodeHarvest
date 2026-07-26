package objectstore

import "testing"

func TestObjectKeyCannotEscapePrefix(t *testing.T) {
	if got := objectKey("tenant/artifacts", "/snapshots/", "../current", "sub.txt"); got != "tenant/artifacts/snapshots/current/sub.txt" {
		t.Fatalf("object key=%q", got)
	}
}
