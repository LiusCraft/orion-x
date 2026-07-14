package handler

import "testing"

func TestContainsFold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  string
		search string
		want   bool
	}{
		{name: "lowercase matches uppercase", value: "CS Memory", search: "cs", want: true},
		{name: "uppercase matches lowercase", value: "device-cs", search: "CS", want: true},
		{name: "unicode case folding", value: "Äpfel Device", search: "äPFEL", want: true},
		{name: "substring absent", value: "memory", search: "device", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := containsFold(tt.value, tt.search); got != tt.want {
				t.Fatalf("containsFold(%q, %q) = %v, want %v", tt.value, tt.search, got, tt.want)
			}
		})
	}
}
