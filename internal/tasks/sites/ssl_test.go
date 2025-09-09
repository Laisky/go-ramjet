package sites_test

import (
	"crypto/tls"
	"fmt"
	"os"
	"testing"
)

func TestSSL(t *testing.T) {
	if os.Getenv("RUN_SSL_IT") == "" {
		t.Skip("integration test disabled: set RUN_SSL_IT to run")
	}
	conn, err := tls.Dial("tcp", "blog.laisky.com:443", nil)
	if err != nil {
		t.Error(err)
	}
	defer conn.Close()

	fmt.Println(conn.ConnectionState().VerifiedChains[0][0].NotAfter)
}
