package handler

import "testing"

func TestSupportsDynamicSplitDomains(t *testing.T) {
	tests := []struct {
		name      string
		license   string
		userAgent string
		want      bool
	}{
		{
			name:      "desktop client",
			license:   "",
			userAgent: "cisco anyconnect vpn agent for mac os x 5.1.16.264",
			want:      true,
		},
		{
			name:      "darwin arm ipad client",
			license:   "mobile",
			userAgent: "anyconnect applesslvpn_darwin_arm (ipad) 5.1.16.264",
			want:      true,
		},
		{
			name:      "android client",
			license:   "mobile",
			userAgent: "anyconnect android 5.1.16.264",
			want:      false,
		},
		{
			name:      "iphone client",
			license:   "mobile",
			userAgent: "cisco anyconnect vpn agent for apple iphone 5.1.16.264",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsDynamicSplitDomains(tt.license, tt.userAgent); got != tt.want {
				t.Fatalf("supportsDynamicSplitDomains(%q, %q) = %v, want %v", tt.license, tt.userAgent, got, tt.want)
			}
		})
	}
}
