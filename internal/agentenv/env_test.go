package agentenv

import (
	"strings"
	"testing"
)

func TestFilterAllowsBasicEnvironmentAndDropsCredentials(t *testing.T) {
	input := []string{
		"PATH=/usr/bin",
		"HOME=/home/symphony",
		"SHELL=/bin/sh",
		"LANG=C.UTF-8",
		"GITEA_TOKEN=<fixture-token>",
		"API_KEY=<fixture-key>",
		"PASSWORD=<fixture-password>",
		"CUSTOM=value",
	}

	got := Filter(input)
	joined := strings.Join(got, "\n")

	for _, want := range []string{"PATH=/usr/bin", "HOME=/home/symphony", "SHELL=/bin/sh", "LANG=C.UTF-8"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Filter() = %#v, missing %s", got, want)
		}
	}
	for _, forbidden := range []string{"GITEA_TOKEN=", "API_KEY=", "PASSWORD=", "CUSTOM="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("Filter() = %#v, leaked %s", got, forbidden)
		}
	}
}
