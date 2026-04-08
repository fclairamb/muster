//go:build manual

package notify

import "testing"

func TestRealOsascript(t *testing.T) {
	n := Notification{
		Title:    "ssf",
		Subtitle: "s/datalake [main]",
		Body:     "if you see this, notifications work",
		Sound:    "Glass",
	}
	if err := (OsascriptNotifier{}).Notify(n); err != nil {
		t.Fatalf("osascript: %v", err)
	}
}

func TestRealBestNotifier(t *testing.T) {
	n := Notification{
		Title:    "ssf",
		Subtitle: "smoke test",
		Body:     "best-notifier path",
		Sound:    "Funk",
		Group:    "ssf-smoketest",
	}
	if err := NewBest().Notify(n); err != nil {
		t.Fatalf("best: %v", err)
	}
}
