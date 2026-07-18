package xiaozhi

import "testing"

func TestValidateManagerURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "http", url: "http://127.0.0.1:9090", want: true},
		{name: "https", url: "https://manager.example.com", want: true},
		{name: "missing", url: "", want: false},
		{name: "relative", url: "/internal", want: false},
		{name: "unsupported scheme", url: "ftp://manager.example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManagerURL(tt.url)
			if (err == nil) != tt.want {
				t.Fatalf("ValidateManagerURL(%q) error = %v, want valid = %v", tt.url, err, tt.want)
			}
		})
	}
}
