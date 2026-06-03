package route

import (
	"testing"

	"github.com/ZihengXiong/GenMult/internal/conversation"
)

func TestDetermineConversationKindTreatsDirectAsDirect(t *testing.T) {
	if got := determineConversationKind("", "direct"); got != conversation.KindDirect {
		t.Fatalf("unexpected conversation kind: %q", got)
	}
}
