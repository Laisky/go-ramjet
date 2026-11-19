package password

import (
	"testing"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/stretchr/testify/require"
)

// TestRunTaskDryModeNeverLogsPassword guards against writing the generated
// Elasticsearch password to the logs during a dry run, which AGENTS.md forbids.
// The dry run must still report that it would change the password for each API.
func TestRunTaskDryModeNeverLogsPassword(t *testing.T) {
	configureTask(t, true, "https://user:pass@es.invalid/_security/user/elastic/_password")
	entries := captureLogs(t)

	password := GeneratePasswdByDate(gutils.UTCNow(), testSecret)
	runTask()

	require.NotEmpty(t, entries.FilterMessage("change password via post").All(), "dry run must still report the change")
	requireNoSecretsLogged(t, entries, password)
}

// TestRunTaskReportsSuccessOnlyAfterARealChange guards the run summary: a dry
// run, or a run with no API configured, changes no password, so it must not
// log "success changed password" and must say why nothing changed instead.
func TestRunTaskReportsSuccessOnlyAfterARealChange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dry     bool
		apis    []string
		summary string
	}{
		{
			name:    "dry run",
			dry:     true,
			apis:    []string{"https://elastic:pass@es.invalid/_security/user/elastic/_password"},
			summary: "dry run finished, no password changed",
		},
		{
			name:    "no api",
			summary: "no elasticsearch api configured, no password changed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureTask(t, tc.dry, tc.apis...)
			entries := captureLogs(t)

			runTask()

			require.Empty(t, entries.FilterMessage("success changed password").All())
			require.Len(t, entries.FilterMessage(tc.summary).All(), 1)
		})
	}
}

// TestRunTaskDryModeNeverLogsAPICredentials guards the admin credentials in the
// API URL. A raw '@' in the password is valid for net/url, so masking must
// parse the URL instead of splitting it on '@'.
func TestRunTaskDryModeNeverLogsAPICredentials(t *testing.T) {
	configureTask(t, true, "https://elastic:"+adminPassword+"@es.invalid/_security/user/elastic/_password")
	entries := captureLogs(t)

	runTask()

	reports := entries.FilterMessage("change password via post").All()
	require.Len(t, reports, 1, "dry run must still report the change")
	require.Contains(t, reports[0].ContextMap()["api"], "es.invalid/_security/user/elastic/_password")
	requireNoSecretsLogged(t, entries, "hunter", "2-secret")
}
