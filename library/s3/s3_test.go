package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	gutils "github.com/Laisky/go-utils/v4"
	gkms "github.com/Laisky/go-utils/v4/crypto/kms"
	kmsMem "github.com/Laisky/go-utils/v4/crypto/kms/mem"
	"github.com/Laisky/testify/require"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Laisky/go-ramjet/library/log"
)

const (
	testS3API    = "http://s3.xego-dev.basebit.me"
	testS3Key    = "pkitest"
	testS3Secret = "pkitest123456"
	testS3Bucket = "pki-test"
)

func newTestCli(t *testing.T, opts ...ClientOption) (ctx context.Context, cli *Client) {
	ctx = context.Background()

	opts = append(opts, WithBucket(testS3Bucket))
	cli, err := NewClient(ctx, testS3API, testS3Key, testS3Secret, opts...)
	require.NoError(t, err)

	return ctx, cli
}

func TestXxx(t *testing.T) {
	ctx, cli := newTestCli(t)

	t.Run("list objects", func(t *testing.T) {
		output, err := cli.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			MaxKeys: aws.Int32(10000),
			Bucket:  aws.String(testS3Bucket),
		})
		require.NoError(t, err)

		for i := range output.Contents {
			t.Logf("got %q", *output.Contents[i].Key)
		}
	})
}

func TestPutEncryptedObject(t *testing.T) {
	kms, err := kmsMem.New(map[uint16][]byte{
		1: []byte("123456"),
	})
	require.NoError(t, err)

	ctx, cli := newTestCli(t)
	require.NoError(t, err)

	key := "test_encrypted_object_" + gutils.UUID7()
	raw, err := gutils.SecRandomBytesWithLength(4 * 1024 * 1024)
	require.NoError(t, err)

	// put object
	_, err = cli.PutObjectEncrypt(ctx, kms, &s3.PutObjectInput{
		Key:  aws.String(key),
		Body: bytes.NewReader(raw),
	})
	require.NoError(t, err)

	// delete object
	defer func() {
		_, _ = cli.DeleteObject(ctx, &s3.DeleteObjectInput{
			Key: aws.String(key),
		})
	}()

	// read object
	t.Run("get encrypted object by http", func(t *testing.T) {
		resp, err := http.Get(fmt.Sprintf("%s/%s/%s", testS3API, testS3Bucket, key))
		require.NoError(t, err)
		defer gutils.LogErr(resp.Body.Close, log.Logger)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		ed := &gkms.EncryptedData{}
		err = ed.Unmarshal(body)
		require.NoError(t, err)

		gotPlain, err := kms.Decrypt(ctx, ed, nil)
		require.NoError(t, err)
		require.Equal(t, raw, gotPlain)
	})

	t.Run("get encrypted object", func(t *testing.T) {
		output, err := cli.GetObjectEncrypt(ctx, kms, &s3.GetObjectInput{
			Key: aws.String(key),
		})
		require.NoError(t, err)
		defer output.Body.Close()

		gotPlain, err := io.ReadAll(output.Body)
		require.NoError(t, err)
		require.Equal(t, raw, gotPlain)
	})
}
