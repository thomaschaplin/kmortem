// Package archiver provides implementations for archiving NodeReports to
// external storage backends.
package archiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/thomaschaplin/kmortem/api/v1alpha1"
	"github.com/thomaschaplin/kmortem/internal/config"
)

// Archiver is implemented by storage backends that can persist a NodeReport.
type Archiver interface {
	Archive(ctx context.Context, report *v1alpha1.NodeReport) error
}

// S3Archiver writes NodeReports as JSON objects to an S3 bucket.
type S3Archiver struct {
	s3client *s3.Client
	cfg      *config.ArchivalConfig
}

// NewS3Archiver creates a new S3Archiver, loading AWS credentials from the
// environment (instance profile, IRSA, env vars, etc.).
func NewS3Archiver(ctx context.Context, cfg *config.ArchivalConfig) (*S3Archiver, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("s3archiver: load AWS config: %w", err)
	}

	return &S3Archiver{
		s3client: s3.NewFromConfig(awsCfg),
		cfg:      cfg,
	}, nil
}

// Archive serialises the NodeReport to JSON and writes it to S3.
// On success it populates report.Spec.Archival with the bucket, key, and
// archival timestamp.
func (a *S3Archiver) Archive(ctx context.Context, report *v1alpha1.NodeReport) error {
	key := a.objectKey(report)

	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("s3archiver: marshal NodeReport %s: %w", report.Name, err)
	}

	_, err = a.s3client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.cfg.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("s3archiver: PutObject s3://%s/%s: %w", a.cfg.Bucket, key, err)
	}

	now := metav1.NewTime(time.Now().UTC())
	report.Spec.Archival = v1alpha1.ArchivalRecord{
		Enabled:    true,
		Bucket:     a.cfg.Bucket,
		Key:        key,
		ArchivedAt: &now,
	}

	return nil
}

// objectKey constructs the S3 key for the NodeReport.
// Format: {prefix}{nodeName}-{nodeUID}.json
func (a *S3Archiver) objectKey(report *v1alpha1.NodeReport) string {
	return fmt.Sprintf("%s%s-%s.json", a.cfg.Prefix, report.Spec.NodeName, report.Spec.NodeUID)
}
