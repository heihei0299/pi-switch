package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeControls(t *testing.T) {
	app := &nativeDemo{}
	tester := ui.NewTester(app.view, 620, 420)
	if err := tester.Click("+"); err != nil {
		t.Fatal(err)
	}
	if app.count != 1 || !tester.HasText("1") {
		t.Fatalf("count %d after click, texts %q", app.count, tester.Texts())
	}
	if err := tester.Click("Name"); err != nil {
		t.Fatal(err)
	}
	tester.Type("Ada")
	if !tester.HasText("Hello, Ada!") {
		t.Fatalf("texts after typing: %q", tester.Texts())
	}
}
