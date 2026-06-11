package server

import "testing"

func TestShouldSkipJWT_ChannelWebhookPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		{path: "/channels/feishu/webhook/cfg-1", want: true},
		{path: "/channels/wechatoa/webhook/cfg-1", want: true},
		{path: "/channels/feishu/webhook", want: false},
		{path: "/api/channels/feishu/webhook", want: false},
	}

	for _, tc := range cases {
		got := shouldSkipJWT(tc.path)
		if got != tc.want {
			t.Fatalf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestRedactURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		uri  string
		want string
	}{
		// JWT via query (WebSocket auth path) must never reach the log.
		{uri: "/bots/b1/container/terminal/ws?token=eyJhbGciOi.payload.sig", want: "/bots/b1/container/terminal/ws?token=%2A%2A%2A"},
		{uri: "/api/thing?api_key=sk-12345&x=1", want: "/api/thing?api_key=%2A%2A%2A&x=1"},
		// No credential params — returned untouched.
		{uri: "/agent-hub/rooms?limit=20", want: "/agent-hub/rooms?limit=20"},
		{uri: "/ping", want: "/ping"},
	}

	for _, tc := range cases {
		if got := redactURI(tc.uri); got != tc.want {
			t.Fatalf("uri=%q want=%q got=%q", tc.uri, tc.want, got)
		}
	}
}
