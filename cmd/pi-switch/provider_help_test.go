package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Review findings (both axes): the commit whose stated purpose was "make the docs
// match reality" left the top-level help LESS complete than before — it dropped
// `duplicate` while adding `use`, still omitted `test|fetch-models|expose` and had
// no trace of `--models`, while `provider --help` never mentioned `--channel`.
// `remove`/`rm`/`ls` work but appear in neither help. README_ZH.md gained a line
// with `--channel` twice.
//
// Hand-checking this drifted twice in one batch, so it is checked mechanically
// from now on (AGENTS.md: rules code can enforce do not belong in prose).

// workingProviderSubcommands is the set the dispatcher really handles: the switch
// in handleProvider plus its aliases. It is written out deliberately — adding a
// subcommand without advertising it in both help texts now fails this test.
var workingProviderSubcommands = []string{
	"list", "ls", "show", "add", "duplicate", "test",
	"fetch-models", "expose", "use", "delete", "remove", "rm",
}

func topLevelHelp(t *testing.T) string {
	t.Helper()
	_, out, _ := runCLIStreams(t, func() int {
		printHelp()
		return 0
	})
	return out
}

func providerHelp(t *testing.T) string {
	t.Helper()
	_, out, _ := runCLIStreams(t, func() int {
		return handleProvider([]string{"--help"})
	})
	return out
}

// H1: every working subcommand is advertised in both help texts, so the help can
// no longer disagree with the dispatcher.
func TestProviderHelp_AdvertisesEveryWorkingSubcommand(t *testing.T) {
	texts := map[string]string{"top-level help": topLevelHelp(t), "provider --help": providerHelp(t)}
	for _, name := range workingProviderSubcommands {
		word := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
		for which, text := range texts {
			if !word.MatchString(text) {
				t.Errorf("%s does not mention the working subcommand %q", which, name)
			}
		}
	}
}

// H2: everything the READMEs advertise really is dispatched. This is the check
// that would have caught the original defect (help advertised commands that
// answered "unknown provider subcommand").
func TestProviderCLI_ReadmeCommandsAreRecognized(t *testing.T) {
	isolateCLI(t)
	re := regexp.MustCompile(`pi-switch provider ([a-z][a-z-]*)`)

	for _, path := range []string{"../../README.md", "../../README_ZH.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		found := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(string(body), -1) {
			found[m[1]] = true
		}
		if len(found) == 0 {
			t.Fatalf("%s: no `pi-switch provider <sub>` examples found; the pattern no longer matches the docs", path)
		}
		for sub := range found {
			_, _, errOut := runCLIStreams(t, func() int {
				return handleProvider([]string{sub, "readme-probe"})
			})
			if strings.Contains(errOut, "unknown provider subcommand") {
				t.Errorf("%s advertises `pi-switch provider %s`, but the CLI does not implement it", path, sub)
			}
		}
	}
}

// H3: no README example passes the same flag twice. README_ZH.md shipped
// `--channel <渠道> --channel main`, where the first value was silently swallowed
// as the flag's argument.
func TestProviderCLI_ReadmeExamplesPassNoFlagTwice(t *testing.T) {
	flagRe := regexp.MustCompile(`--[a-z-]+`)

	for _, path := range []string{"../../README.md", "../../README_ZH.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, "pi-switch provider expose") {
				continue
			}
			seen := map[string]int{}
			for _, f := range flagRe.FindAllString(line, -1) {
				seen[f]++
			}
			for flag, n := range seen {
				if n > 1 {
					t.Errorf("%s:%d passes %s %d times: %s", path, i+1, flag, n, strings.TrimSpace(line))
				}
			}
		}
	}
}
