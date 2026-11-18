package ark

import (
	"testing"

	"starhub/relay/channel"
)

func TestArkEndpoints(t *testing.T) {
	a := Adaptor{}
	for _, base := range []string{"https://example.test/prefix", "https://example.test/prefix/api/v3/"} {
		if got := a.ChatURL(base, "", "", false); got != "https://example.test/prefix/api/v3/chat/completions" {
			t.Fatal(got)
		}
		for mode, path := range map[channel.RelayMode]string{channel.RelayModeImageGeneration: "/images/generations", channel.RelayModeEmbeddings: "/embeddings", channel.RelayModeResponses: "/responses"} {
			if got, err := a.ModeURL(base, mode); err != nil || got != "https://example.test/prefix/api/v3"+path {
				t.Fatalf("unexpected endpoint %s: %v", got, err)
			}
		}
	}
	if APIBaseURL("") != DefaultBaseURL+"/api/v3" {
		t.Fatal("missing default Ark endpoint")
	}
	for _, mode := range []channel.RelayMode{channel.RelayModeImageEdit, channel.RelayModeTTS, channel.RelayModeRerank} {
		if _, err := a.ModeURL("", mode); err != channel.ErrUnsupportedMode {
			t.Fatal("unsupported mode was advertised")
		}
	}
	if _, err := a.RealtimeURL("", ""); err != channel.ErrUnsupportedMode {
		t.Fatal("unsupported realtime was advertised")
	}
}
