package sites_test

import (
	"crypto/tls"
	"fmt"
	"testing"
)

func TestSSL(t *testing.T) {
	conn, err := tls.Dial("tcp", "blog.laisky.com:443", nil)
	if err != nil {
		t.Error(err)
	}
	defer conn.Close()

	fmt.Println(conn.ConnectionState().VerifiedChains[0][0].NotAfter)

	t.Error("done")
}
