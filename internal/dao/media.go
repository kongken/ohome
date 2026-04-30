package dao

import (
	bs3 "butterfly.orx.me/core/store/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// MediaBucket is the configured S3 bucket name for user-uploaded media
// (avatars, covers, post images/videos, message attachments, photos).
// Configure under `store.s3.media` in config.yaml.
const MediaBucket = "media"

// MediaClient returns the S3 client for the media bucket.
func MediaClient() *s3.Client {
	return bs3.GetClient(MediaBucket)
}

// MediaBucketName returns the configured bucket name (so callers don't have
// to hardcode it).
func MediaBucketName() string {
	return bs3.GetBucket(MediaBucket)
}
