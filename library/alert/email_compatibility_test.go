package alert

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gconfig "github.com/Laisky/go-config/v2"
	emailSDK "github.com/Laisky/go-utils/v6/email"
	"github.com/stretchr/testify/require"
)

// TestEmailSendRequiresTLS checks the actual alert wrapper against a loopback
// SMTP peer without STARTTLS and requires a wrapped failure before message data.
func TestEmailSendRequiresTLS(t *testing.T) {
	configureSyntheticEmail(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })
	received := make(chan string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			received <- acceptErr.Error()
			return
		}
		defer conn.Close()
		if deadlineErr := conn.SetDeadline(time.Now().Add(5 * time.Second)); deadlineErr != nil {
			received <- deadlineErr.Error()
			return
		}
		writer := bufio.NewWriter(conn)
		if _, writeErr := fmt.Fprint(writer, "220 synthetic fixture SMTP\r\n"); writeErr != nil {
			received <- writeErr.Error()
			return
		}
		if flushErr := writer.Flush(); flushErr != nil {
			received <- flushErr.Error()
			return
		}
		reader := bufio.NewReader(conn)
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			received <- readErr.Error()
			return
		}
		transcript := line
		if !strings.HasPrefix(line, "EHLO ") {
			received <- transcript
			return
		}
		if _, writeErr := fmt.Fprint(writer, "250-synthetic fixture\r\n250 HELP\r\n"); writeErr != nil {
			received <- writeErr.Error()
			return
		}
		if flushErr := writer.Flush(); flushErr != nil {
			received <- flushErr.Error()
			return
		}
		next, _ := reader.ReadString('\n')
		received <- transcript + next
	}()
	previousDry := gconfig.Shared.GetBool("dry")
	gconfig.Shared.Set("dry", false)
	t.Cleanup(func() { gconfig.Shared.Set("dry", previousDry) })
	sender := &EmailType{sender: emailSDK.NewMail("127.0.0.1", listener.Addr().(*net.TCPAddr).Port)}
	err = sender.Send("recipient@example.invalid", "Synthetic recipient", "synthetic marker", "synthetic body")
	require.Error(t, err)
	require.Contains(t, err.Error(), "go-ramjet try to send email")
	require.Contains(t, strings.ToLower(err.Error()), "starttls")
	select {
	case transcript := <-received:
		require.True(t, strings.HasPrefix(transcript, "EHLO "), transcript)
		require.NotContains(t, transcript, "MAIL FROM")
		require.NotContains(t, transcript, "DATA")
	case <-time.After(6 * time.Second):
		t.Fatal("synthetic SMTP fixture did not finish")
	}
}

// fixtureTLSMail adds fixture CA trust while retaining the production Send call shape.
type fixtureTLSMail struct {
	*emailSDK.MailT
	tlsConfig *tls.Config
}

// Send delegates the alert message through verified TLS with the synthetic CA.
func (m *fixtureTLSMail) Send(from, to, fromName, toName, subject, body string, opts ...emailSDK.SendOption) error {
	return m.MailT.Send(from, to, fromName, toName, subject, body, append(opts, emailSDK.WithEmailRequireTLS(m.tlsConfig))...)
}

// TestEmailSendLOGINCompatibility exercises actual alert delivery to a verified
// STARTTLS peer advertising only LOGIN, with a rejected-password control.
func TestEmailSendLOGINCompatibility(t *testing.T) {
	configureSyntheticEmail(t)
	for _, accepted := range []bool{true, false} {
		t.Run(fmt.Sprintf("accepted=%t", accepted), func(t *testing.T) {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic SMTP"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
			der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
			require.NoError(t, err)
			cert, err := x509.ParseCertificate(der)
			require.NoError(t, err)
			roots := x509.NewCertPool()
			roots.AddCert(cert)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, listener.Close()) })
			result := make(chan string, 1)
			go serveLOGINFixture(listener, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, accepted, result)
			mail := &fixtureTLSMail{MailT: emailSDK.NewMail("127.0.0.1", listener.Addr().(*net.TCPAddr).Port), tlsConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
			mail.Login("synthetic-user", "synthetic-password")
			previousDry := gconfig.Shared.GetBool("dry")
			gconfig.Shared.Set("dry", false)
			t.Cleanup(func() { gconfig.Shared.Set("dry", previousDry) })
			sender := &EmailType{sender: mail}
			err = sender.Send("recipient@example.invalid", "Synthetic recipient", "synthetic marker", "synthetic body")
			if accepted {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "535")
			}
			select {
			case outcome := <-result:
				if accepted {
					require.Equal(t, "delivered", outcome)
				} else {
					require.Equal(t, "rejected", outcome)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("synthetic LOGIN fixture did not finish")
			}
		})
	}
}

// serveLOGINFixture runs one bounded loopback SMTP session and reports delivery
// only after verified TLS, LOGIN challenges, and complete message data.
func serveLOGINFixture(listener net.Listener, certificate tls.Certificate, accepted bool, result chan<- string) {
	conn, err := listener.Accept()
	if err != nil {
		result <- err.Error()
		return
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		result <- err.Error()
		return
	}
	reader := bufio.NewReader(conn)
	write := func(text string) error { _, err := fmt.Fprint(conn, text); return err }
	if err := write("220 synthetic SMTP\r\n"); err != nil {
		result <- err.Error()
		return
	}
	tlsReady := false
	dataReady := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if dataReady && errors.Is(err, io.EOF) {
				result <- "delivered"
			} else {
				result <- err.Error()
			}
			return
		}
		command := strings.TrimSpace(line)
		response := "250 OK\r\n"
		switch {
		case strings.HasPrefix(command, "EHLO "):
			response = "250-synthetic\r\n250 STARTTLS\r\n"
			if tlsReady {
				response = "250-synthetic\r\n250 AUTH LOGIN\r\n"
			}
		case command == "STARTTLS":
			if err := write("220 upgrade\r\n"); err != nil {
				result <- err.Error()
				return
			}
			secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
			if err := secured.Handshake(); err != nil {
				result <- err.Error()
				return
			}
			conn = secured
			reader = bufio.NewReader(conn)
			tlsReady = true
			continue
		case command == "AUTH LOGIN" && tlsReady:
			if err := write("334 VXNlcm5hbWU6\r\n"); err != nil {
				result <- err.Error()
				return
			}
			username, err := reader.ReadString('\n')
			if err != nil {
				result <- err.Error()
				return
			}
			if err := write("334 UGFzc3dvcmQ6\r\n"); err != nil {
				result <- err.Error()
				return
			}
			password, err := reader.ReadString('\n')
			if err != nil {
				result <- err.Error()
				return
			}
			valid := strings.TrimSpace(username) == base64.StdEncoding.EncodeToString([]byte("synthetic-user")) && strings.TrimSpace(password) == base64.StdEncoding.EncodeToString([]byte("synthetic-password"))
			if !accepted || !valid {
				if err := write("535 synthetic rejected\r\n"); err != nil {
					result <- err.Error()
					return
				}
				result <- "rejected"
				return
			}
			response = "235 authenticated\r\n"
		case strings.HasPrefix(command, "AUTH "):
			if err := write("504 unsupported mechanism\r\n"); err != nil {
				result <- err.Error()
				return
			}
			result <- "unsupported auth"
			return
		case command == "DATA":
			if err := write("354 enter data\r\n"); err != nil {
				result <- err.Error()
				return
			}
			body, err := textproto.NewReader(reader).ReadDotBytes()
			if err != nil {
				result <- err.Error()
				return
			}
			dataReady = strings.Contains(string(body), "synthetic marker")
		case command == "QUIT":
			if err := write("221 bye\r\n"); err != nil {
				result <- err.Error()
				return
			}
			if dataReady {
				result <- "delivered"
			} else {
				result <- "missing data"
			}
			return
		}
		if err := write(response); err != nil {
			result <- err.Error()
			return
		}
	}
}

// configureSyntheticEmail sets only in-process fixture configuration and restores
// the prior values when the test completes.
func configureSyntheticEmail(t *testing.T) {
	t.Helper()
	sender := gconfig.Shared.Get("email.sender")
	senderName := gconfig.Shared.Get("email.senderName")
	gconfig.Shared.Set("email.sender", "sender@example.invalid")
	gconfig.Shared.Set("email.senderName", "Synthetic sender")
	t.Cleanup(func() { gconfig.Shared.Set("email.sender", sender); gconfig.Shared.Set("email.senderName", senderName) })
}
