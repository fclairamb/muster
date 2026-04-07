//go:build manual

package notify

import "testing"

func TestRealOsascript(t *testing.T) {
	if err := (OsascriptNotifier{}).Notify("ssf test", "if you see this, notifications work"); err != nil {
		t.Fatalf("osascript: %v", err)
	}
}
