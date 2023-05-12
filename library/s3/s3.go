// Package s3 s3 client
package s3

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v4"
	gkms "github.com/Laisky/go-utils/v4/crypto/kms"
	glog "github.com/Laisky/go-utils/v4/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultRegion = "us-east-1"
)

// Client aws s3 client
type Client struct {
	opt *clientOption
	*s3.Client
}

type clientOption struct {
	logger         glog.Logger
	bucket, region string
	debugLog       bool
}

// ClientOption optional arguments for client
type ClientOption func(*clientOption) error

func (o *clientOption) fillDefault() *clientOption {
	o.region = defaultRegion
	o.logger = glog.Shared.Named("s3_cli")
	return o
}

func (o *clientOption) applyOpts(opts ...ClientOption) (*clientOption, error) {
	for i := range opts {
		if err := opts[i](o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// WithBucket set default bucket
func WithBucket(bucket string) ClientOption {
	return func(o *clientOption) error {
		o.bucket = bucket
		return nil
	}
}

// WithRegion set default region
func WithRegion(region string) ClientOption {
	return func(o *clientOption) error {
		o.region = region
		return nil
	}
}

// WithDebugLog enable debug log
func WithDebugLog() ClientOption {
	return func(o *clientOption) error {
		o.debugLog = true
		return nil
	}
}

// NewClient new s3 client
//
// if set kms engine by WithKMS, ListObject/GetObject will enable transparent encryption.
func NewClient(ctx context.Context, endpoint, key, secret string, opts ...ClientOption) (*Client, error) {
	opt, err := new(clientOption).fillDefault().applyOpts(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "apply options")
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				SigningRegion: region,
				URL:           endpoint,
			}, nil
		})

	s3Opts := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(key, secret, "")),
		config.WithDefaultRegion(opt.region),
		config.WithEndpointResolverWithOptions(customResolver),
	}
	if opt.debugLog {
		s3Opts = append(s3Opts,
			config.WithClientLogMode(aws.LogRequestWithBody|aws.LogResponseWithBody))
	}

	cfg, err := config.LoadDefaultConfig(ctx, s3Opts...)
	if err != nil {
		return nil, errors.Wrap(err, "load s3 config")
	}

	cli := &Client{
		opt: opt,
		Client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
		}),
	}

	return cli, nil
}

// Bucket return bucket name
func (c *Client) Bucket() string {
	return c.opt.bucket
}

// Region return region name
func (c *Client) Region() string {
	return c.opt.region
}

// ErrNotFound check error is not found
func (c *Client) ErrNotFound(err error) bool {
	if err != nil {
		var responseError *awshttp.ResponseError
		if errors.As(err, &responseError) &&
			responseError.ResponseError.HTTPStatusCode() == http.StatusNotFound {
			return true
		}
	}

	return false
}

// IsObjectExist check object exist
func (c *Client) IsObjectExist(ctx context.Context, key string) (bool, error) {
	_, err := c.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.opt.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if c.ErrNotFound(err) {
			return false, nil
		}

		return false, errors.Wrap(err, "get object")
	}

	return true, nil
}

// ListObjectsV2 list object
func (c *Client) ListObjectsV2(ctx context.Context,
	params *s3.ListObjectsV2Input,
	optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	return c.Client.ListObjectsV2(ctx, params, optFns...)
}

// PutObject put s3 object
func (c *Client) PutObject(ctx context.Context,
	params *s3.PutObjectInput,
	optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	return c.Client.PutObject(ctx, params, optFns...)
}

// DeleteObject delete s3 object
func (c *Client) DeleteObject(ctx context.Context,
	params *s3.DeleteObjectInput,
	optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	return c.Client.DeleteObject(ctx, params, optFns...)
}

// GetObject get s3 object
func (c *Client) GetObject(ctx context.Context,
	params *s3.GetObjectInput,
	optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	output, err := c.Client.GetObject(ctx, params, optFns...)
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}

	return output, nil
}

// PutObjectEncrypt put s3 object
func (c *Client) PutObjectEncrypt(ctx context.Context,
	kms KMS,
	params *s3.PutObjectInput,
	optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	cnt, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}

	ed, err := kms.Encrypt(ctx, cnt, nil)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt content")
	}

	cipherBody, err := ed.Marshal()
	if err != nil {
		return nil, errors.Wrap(err, "marshal encrypted data")
	}

	params.Body = bytes.NewReader(cipherBody)
	return c.PutObject(ctx, params, optFns...)
}

// GetObjectEncrypt get s3 object
func (c *Client) GetObjectEncrypt(ctx context.Context,
	kms KMS,
	params *s3.GetObjectInput,
	optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if gutils.IsEmpty(params.Bucket) {
		params.Bucket = aws.String(c.opt.bucket)
	}

	output, err := c.GetObject(ctx, params, optFns...)
	if err != nil {
		return nil, errors.Wrap(err, "get object")
	}
	defer gutils.SilentClose(output.Body)

	cipherBody, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read encrypt body")
	}

	ed := &gkms.EncryptedData{}
	if err := ed.Unmarshal(cipherBody); err != nil {
		return nil, errors.Wrap(err, "unmarshal encrypted object")
	}

	plain, err := kms.Decrypt(ctx, ed, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt object")
	}

	output.Body = io.NopCloser(bytes.NewReader(plain))
	return output, nil
}
