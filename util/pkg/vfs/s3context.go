/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vfs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyendpoints "github.com/aws/smithy-go/endpoints"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/klog/v2"
)

// matches regional naming conventions of S3:
// https://docs.aws.amazon.com/general/latest/gr/s3.html
// TODO: match fips and S3 access point naming conventions
// TODO: perhaps make region regex more specific, i.e. (us|eu|ap|cn|ca|sa), to prevent matching bucket names that match region format?
//
//	but that will mean updating this list when AWS introduces new regions
var s3UrlRegexp = regexp.MustCompile(`(s3([-.](?P<region>\w{2}(-gov)?-\w+-\d{1})|[-.](?P<bucket>[\w.\-\_]+)|)?|(?P<bucket>[\w.\-\_]+)[.]s3([.-](?P<region>\w{2}(-gov)?-\w+-\d{1}))?)[.]amazonaws[.]com([.]cn)?(?P<path>.*)?`)

type S3BucketDetails struct {
	// context is the S3Context we are associated with
	context *S3Context

	// region is the region we have determined for the bucket
	region string

	// name is the name of the bucket
	name string

	// mutex protects applyServerSideEncryptionByDefault
	mutex sync.Mutex

	// applyServerSideEncryptionByDefault caches information on whether server-side encryption is enabled on the bucket
	applyServerSideEncryptionByDefault *bool
}

type S3Context struct {
	mutex         sync.Mutex
	clients       map[string]*s3.Client
	bucketDetails map[string]*S3BucketDetails
}

func NewS3Context() *S3Context {
	return &S3Context{
		clients:       make(map[string]*s3.Client),
		bucketDetails: make(map[string]*S3BucketDetails),
	}
}

type ResolverV2 struct{}

func (*ResolverV2) ResolveEndpoint(ctx context.Context, params s3.EndpointParameters) (
	smithyendpoints.Endpoint, error,
) {
	params.UseDualStack = aws.Bool(true)
	return s3.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, params)
}

func (s *S3Context) getClient(ctx context.Context, region string, optFn func(*s3.Options)) (*s3.Client, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s3Client := s.clients[region]; s3Client != nil {
		return s3Client, nil
	}

	// Client configuration is determined by region and process-wide environment.
	// The first request for a region creates the shared client for that region.
	_, span := tracer.Start(ctx, "S3Context::getClient")
	defer span.End()

	var config aws.Config
	var err error
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		config, err = awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("error loading AWS config: %v", err)
		}
	} else {
		// Use customized S3 storage
		klog.V(2).Infof("Found S3_ENDPOINT=%q, using as non-AWS S3 backend", endpoint)
		config, err = getCustomS3Config(ctx, region)
		if err != nil {
			return nil, err
		}
	}

	s3Client := s3.NewFromConfig(config, optFn)

	s.clients[region] = s3Client

	return s3Client, nil
}

func getCustomS3Config(ctx context.Context, region string) (aws.Config, error) {
	accessKeyID := os.Getenv("S3_ACCESS_KEY_ID")
	if accessKeyID == "" {
		return aws.Config{}, fmt.Errorf("S3_ACCESS_KEY_ID cannot be empty when S3_ENDPOINT is not empty")
	}
	secretAccessKey := os.Getenv("S3_SECRET_ACCESS_KEY")
	if secretAccessKey == "" {
		return aws.Config{}, fmt.Errorf("S3_SECRET_ACCESS_KEY cannot be empty when S3_ENDPOINT is not empty")
	}

	s3Config, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("error loading AWS config: %v", err)
	}
	return s3Config, nil
}

func (s *S3Context) getDetailsForBucket(ctx context.Context, bucket string) (*S3BucketDetails, error) {
	s.mutex.Lock()
	bucketDetails := s.bucketDetails[bucket]
	s.mutex.Unlock()

	if bucketDetails != nil && bucketDetails.region != "" {
		return bucketDetails, nil
	}

	ctx, span := tracer.Start(ctx, "S3Path::getDetailsForBucket")
	defer span.End()

	bucketDetails = &S3BucketDetails{
		context: s,
		region:  "",
		name:    bucket,
	}

	// Probe to find correct region for bucket
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint != "" {
		// If customized S3 storage is set, return user-defined region
		bucketDetails.region = os.Getenv("S3_REGION")
		if bucketDetails.region == "" {
			bucketDetails.region = "us-east-1"
		}
		return bucketDetails, nil
	}

	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		isEC2, err := isRunningOnEC2(ctx)
		if isEC2 || err != nil {
			region, err := getRegionFromMetadata(ctx)
			if err != nil {
				klog.V(2).Infof("unable to get region from metadata:%v", err)
			} else {
				awsRegion = region
				klog.V(2).Infof("got region from metadata: %q", awsRegion)
			}
		}
	}

	if awsRegion == "" {
		awsRegion = "us-east-1"
		klog.V(2).Infof("defaulting region to %q", awsRegion)
	}

	s3Client, err := s.getClient(ctx, awsRegion, func(o *s3.Options) {
		o.EndpointResolverV2 = &ResolverV2{}
	})
	if err != nil {
		return bucketDetails, fmt.Errorf("error connecting to S3: %s", err)
	}
	// Attempt one GetBucketLocation call the "normal" way (i.e. as the bucket owner)
	response, err := s3Client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: &bucket,
	})

	if err != nil {
		// GetBucketLocation only works for the bucket owner from any region, or from the bucket's
		// region. Fall back to HeadBucket, which works cross-account and cross-region.
		klog.V(2).Infof("unable to get bucket location from region %q; falling back to HeadBucket: %v", awsRegion, err)
		bucketDetails.region, err = bucketLocationViaHead(ctx, s3Client, bucket)
		if err != nil {
			return bucketDetails, err
		}
	} else if len(response.LocationConstraint) == 0 {
		// US Classic does not return a region
		bucketDetails.region = "us-east-1"
	} else {
		bucketDetails.region = string(response.LocationConstraint)
		// Another special case: "EU" can mean eu-west-1
		if bucketDetails.region == "EU" {
			bucketDetails.region = "eu-west-1"
		}
	}

	klog.V(2).Infof("found bucket in region %q", bucketDetails.region)

	s.mutex.Lock()
	s.bucketDetails[bucket] = bucketDetails
	s.mutex.Unlock()

	return bucketDetails, nil
}

func (b *S3BucketDetails) hasServerSideEncryptionByDefault(ctx context.Context, optFn func(*s3.Options)) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.applyServerSideEncryptionByDefault != nil {
		return *b.applyServerSideEncryptionByDefault
	}

	ctx, span := tracer.Start(ctx, "S3BucketDetails::hasServerSideEncryptionByDefault")
	defer span.End()

	applyServerSideEncryptionByDefault := false

	// We only make one attempt to find the SSE policy (even if there's an error)
	b.applyServerSideEncryptionByDefault = &applyServerSideEncryptionByDefault

	client, err := b.context.getClient(ctx, b.region, optFn)
	if err != nil {
		klog.Warningf("Unable to read bucket encryption policy for %q in region %q: will encrypt using AES256", b.name, b.region)
		return false
	}

	klog.V(4).Infof("Checking default bucket encryption for %q", b.name)

	request := &s3.GetBucketEncryptionInput{}
	request.Bucket = aws.String(b.name)

	klog.V(8).Infof("Calling S3 GetBucketEncryption Bucket=%q", b.name)

	result, err := client.GetBucketEncryption(ctx, request)
	if err != nil {
		// the following cases might lead to the operation failing:
		// 1. A deny policy on s3:GetEncryptionConfiguration
		// 2. No default encryption policy set
		klog.V(8).Infof("Unable to read bucket encryption policy for %q: will encrypt using AES256", b.name)
		return false
	}

	// currently, only one element is in the rules array, iterating nonetheless for future compatibility
	for _, element := range result.ServerSideEncryptionConfiguration.Rules {
		if element.ApplyServerSideEncryptionByDefault != nil {
			applyServerSideEncryptionByDefault = true
		}
	}

	b.applyServerSideEncryptionByDefault = &applyServerSideEncryptionByDefault

	klog.V(2).Infof("bucket %q has default encryption set to %t", b.name, applyServerSideEncryptionByDefault)

	return applyServerSideEncryptionByDefault
}

// bucketLocationViaHead resolves the region for a bucket using HeadBucket.
// GetBucketLocation does not work for cross-account buckets queried from a different region.
// HeadBucket can be called against any region in the partition: on success the response carries
// BucketRegion, and on a cross-region failure (e.g. 301 PermanentRedirect) S3 still sets the
// x-amz-bucket-region response header, which we recover from the wrapped response error.
func bucketLocationViaHead(ctx context.Context, s3Client *s3.Client, bucket string) (string, error) {
	ctx, span := tracer.Start(ctx, "bucketLocationViaHead")
	defer span.End()

	out, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		if out.BucketRegion != nil && *out.BucketRegion != "" {
			return *out.BucketRegion, nil
		}
		return "", fmt.Errorf("HeadBucket on %q did not return a bucket region", bucket)
	}

	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil && respErr.Response.Response != nil {
		if bucketRegion := respErr.Response.Header.Get("x-amz-bucket-region"); bucketRegion != "" {
			return bucketRegion, nil
		}
	}
	return "", fmt.Errorf("getting location for bucket %q: %w", bucket, err)
}

// isRunningOnEC2 determines if we could be running on EC2.
// It is used to avoid a call to the metadata service to get the current region,
// because that call is slow if not running on EC2
func isRunningOnEC2(ctx context.Context) (bool, error) {
	if runtime.GOOS == "linux" {
		// Approach based on https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/identify_ec2_instances.html
		productUUID, err := os.ReadFile("/sys/devices/virtual/dmi/id/product_uuid")
		if err != nil {
			klog.V(2).Infof("unable to read /sys/devices/virtual/dmi/id/product_uuid, assuming not running on EC2: %v", err)
			return false, nil
		}

		s := strings.ToLower(strings.TrimSpace(string(productUUID)))
		if strings.HasPrefix(s, "ec2") {
			klog.V(2).Infof("product_uuid is %q, assuming running on EC2", s)
			return true, nil
		}
		klog.V(2).Infof("product_uuid is %q, assuming not running on EC2", s)
		return false, nil
	}
	klog.V(2).Infof("GOOS=%q, assuming not running on EC2", runtime.GOOS)
	return false, nil
}

// getRegionFromMetadata queries the metadata service for the current region, if running in EC2
func getRegionFromMetadata(ctx context.Context) (string, error) {
	ctx, span := tracer.Start(ctx, "getRegionFromMetadata")
	defer span.End()

	// Use an even shorter timeout, to minimize impact when not running on EC2
	// Note that we still retry a few times, this works out a little under a 1s delay
	shortTimeout := &http.Client{
		Timeout: 100 * time.Millisecond,
	}

	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithHTTPClient(shortTimeout))
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := imds.NewFromConfig(config)

	metadataRegion, err := client.GetRegion(ctx, &imds.GetRegionInput{})
	if err != nil {
		return "", fmt.Errorf("getting AWS region from metadata: %w", err)
	}

	return metadataRegion.Region, nil
}

func VFSPath(url string) (string, error) {
	if !s3UrlRegexp.MatchString(url) {
		return "", fmt.Errorf("%s is not a valid S3 URL", url)
	}
	groupNames := s3UrlRegexp.SubexpNames()
	result := s3UrlRegexp.FindAllStringSubmatch(url, -1)[0]

	captured := map[string]string{}
	for i, value := range result {
		if value != "" {
			captured[groupNames[i]] = value
		}
	}
	bucket := captured["bucket"]
	path := captured["path"]
	if bucket == "" {
		if path == "" {
			return "", fmt.Errorf("%s is not a valid S3 URL. No bucket defined.", url)
		}
		return fmt.Sprintf("s3:/%s", path), nil
	}
	return fmt.Sprintf("s3://%s%s", bucket, path), nil
}
