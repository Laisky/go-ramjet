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
