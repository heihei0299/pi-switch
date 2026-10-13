package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeJSONDraftPastesClipboardOverSelection(t *testing.T) {
	preview := newNativeComplexPreview()
	tester := ui.NewTester(preview.view, 900, 720)
	if err := tester.Click("Draft JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	pasted := `{"providers":{}}`
	tester.SetClipboard(pasted)
	tester.Key(ui.Cmd, ui.KeyV)
	if preview.jsonDraft != pasted {
		t.Fatalf("pasted JSON draft = %q, want %q", preview.jsonDraft, pasted)
	}
}
