package notify

import "testing"

type recorder struct {
	title, body string
	err         error
}

func (r *recorder) Notify(title, body string) error {
	r.title, r.body = title, body
	return r.err
}

func TestNopNeverFails(t *testing.T) {
	if err := Nop.Notify("t", "b"); err != nil {
		t.Fatalf("Nop.Notify() = %v", err)
	}
}

func TestNotifierInterface(t *testing.T) {
	tests := []struct {
		name  string
		title string
		body  string
	}{
		{"uploaded", "Ride uploaded to Garmin", "a.fit"},
		{"failed", "Upload failed", "a.fit: boom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{}
			var n Notifier = r
			if err := n.Notify(tc.title, tc.body); err != nil {
				t.Fatal(err)
			}
			if r.title != tc.title || r.body != tc.body {
				t.Errorf("got %q/%q, want %q/%q", r.title, r.body, tc.title, tc.body)
			}
		})
	}
}
